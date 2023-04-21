package soap

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
)

// OperationHandlerFunc runs the actual business logic - request is whatever you constructed in RequestFactoryFunc
type OperationHandlerFunc func(request interface{}, w http.ResponseWriter, httpRequest *http.Request) (response interface{}, err error)

// RequestFactoryFunc constructs a request object for OperationHandlerFunc
type RequestFactoryFunc func() interface{}

type dummyContent struct{}

type operationHandler struct {
	requestFactory RequestFactoryFunc
	handler        OperationHandlerFunc
}

type responseWriter struct {
	log           func(...interface{})
	w             http.ResponseWriter
	outputStarted bool
}

func (w *responseWriter) Header() http.Header {
	return w.w.Header()
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.outputStarted = true
	if w.log != nil {
		w.log("writing response: ", string(b))
	}
	return w.w.Write(b)
}

func (w *responseWriter) WriteHeader(code int) {
	w.w.WriteHeader(code)
}

// Server a SOAP server, which can be run standalone or used as a http.HandlerFunc
type Server struct {
	Log         func(...interface{}) // do nothing on nil or add your fmt.Print* or log.*
	handlers    map[string]map[string]map[string]*operationHandler
	Marshaller  XMLMarshaller
	ContentType string
	SoapVersion string
}

// NewServer construct a new SOAP server
func NewServer() *Server {
	return &Server{
		handlers:    make(map[string]map[string]map[string]*operationHandler),
		Marshaller:  defaultMarshaller{},
		ContentType: SoapContentType11,
		SoapVersion: SoapVersion11,
	}
}

func (s *Server) log(args ...interface{}) {
	if s.Log != nil {
		s.Log(args...)
	}
}

func (s *Server) UseSoap11() {
	s.SoapVersion = SoapVersion11
	s.ContentType = SoapContentType11
}

func (s *Server) UseSoap12() {
	s.SoapVersion = SoapVersion12
	s.ContentType = SoapContentType12
}

// RegisterHandler register to handle an operation. This function must not be
// called after the server has been started.
func (s *Server) RegisterHandler(path string, action string, messageType string, requestFactory RequestFactoryFunc, operationHandlerFunc OperationHandlerFunc) {
	if _, ok := s.handlers[path]; !ok {
		s.handlers[path] = make(map[string]map[string]*operationHandler)
	}

	if _, ok := s.handlers[path][action]; !ok {
		s.handlers[path][action] = make(map[string]*operationHandler)
	}
	s.handlers[path][action][messageType] = &operationHandler{
		handler:        operationHandlerFunc,
		requestFactory: requestFactory,
	}
}

func (s *Server) handleError(err error, w http.ResponseWriter) {
	// has to write a soap fault
	s.log("handling error:", err)
	responseEnvelope := &Envelope{
		Body: Body{
			Content: &Fault{
				String: err.Error(),
			},
		},
	}
	xmlBytes, xmlErr := s.Marshaller.Marshal(responseEnvelope)
	if xmlErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "could not marshal soap fault for: %s xmlError: %s\n", err, xmlErr)
		return
	}
	addSOAPHeader(w, len(xmlBytes), s.ContentType)
	w.Write(xmlBytes)
}

// WriteHeader first set the content-type header and then writes the header code.
func (s *Server) WriteHeader(w http.ResponseWriter, code int) {
	setContentType(w, s.ContentType)
	w.WriteHeader(code)
}

func setContentType(w http.ResponseWriter, contentType string) {
	w.Header().Set("Content-Type", contentType)
}

func addSOAPHeader(w http.ResponseWriter, contentLength int, contentType string) {
	setContentType(w, contentType)
	w.Header().Set("Content-Length", fmt.Sprint(contentLength))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// we have a valid request time to call the handler
	w = &responseWriter{
		log:           s.Log,
		w:             w,
		outputStarted: false,
	}
	switch r.Method {
	case "POST":
		soapRequestBytes, err := ioutil.ReadAll(r.Body)
		// Our structs for Envelope, Header, Body and Fault are tagged with namespace for SOAP 1.1
		// Therefore we must adjust namespaces for incoming SOAP 1.2 messages
		if s.SoapVersion == SoapVersion12 {
			soapRequestBytes = replaceSoap12to11(soapRequestBytes)
		}

		if err != nil {
			s.handleError(fmt.Errorf("could not read POST:: %s", err), w)
			return
		}
		pathHandlers, ok := s.handlers[r.URL.Path]
		if !ok {
			s.handleError(fmt.Errorf("unknown path %q", r.URL.Path), w)
			return
		}

		// we need to find out, what is in the body
		probeEnvelope := &Envelope{
			Body: Body{
				Content: &dummyContent{},
			},
		}

		if err := s.Marshaller.Unmarshal(soapRequestBytes, probeEnvelope); err != nil {
			s.handleError(fmt.Errorf("could not probe soap body content:: %s", err), w)
			return
		}
		t := probeEnvelope.Body.SOAPBodyContentType
		s.log("found content type", t)

		// Look for SOAPAction in header first
		soapAction := r.Header.Get("SOAPAction")
		actionHandlers, ok := pathHandlers[soapAction]
		if !ok {
			// look at content type
			soapAction = t
			actionHandlers, ok = pathHandlers[soapAction]
			if !ok {
				s.handleError(fmt.Errorf("unknown action %q", soapAction), w)
				return
			}
		}
		s.log("ServeHTTP method:", r.Method, ", path:", r.URL.Path, ", SOAPAction:", soapAction)

		actionHandler, ok := actionHandlers[t]
		if !ok {
			s.handleError(fmt.Errorf("no action handler for content type: %q", t), w)
			return
		}
		request := actionHandler.requestFactory()
		envelope := &Envelope{
			Header: Header{},
			Body: Body{
				Content: request,
			},
		}

		if err := xml.Unmarshal(soapRequestBytes, &envelope); err != nil {
			s.handleError(fmt.Errorf("could not unmarshal request:: %s", err), w)
			return
		}
		s.log("request", s.jsonDump(envelope))

		response, err := actionHandler.handler(request, w, r)
		if err != nil {
			s.log("action handler threw up")
			s.handleError(err, w)
			return
		}
		s.log("result", s.jsonDump(response))
		if !w.(*responseWriter).outputStarted {
			responseEnvelope := &Envelope{
				Body: Body{
					Content: response,
				},
			}
			xmlBytes, err := s.Marshaller.Marshal(responseEnvelope)
			// Adjust namespaces for SOAP 1.2
			if s.SoapVersion == SoapVersion12 {
				xmlBytes = replaceSoap11to12(xmlBytes)
			}
			if err != nil {
				s.handleError(fmt.Errorf("could not marshal response:: %s", err), w)
			}
			addSOAPHeader(w, len(xmlBytes), s.ContentType)
			w.Write(xmlBytes)
		} else {
			s.log("action handler sent its own output")
		}

	default:
		// this will be a soap fault !?
		s.handleError(errors.New("this is a soap service - you have to POST soap requests"), w)
	}
}

func (s *Server) jsonDump(v interface{}) string {
	if s.Log == nil {
		return "not dumping"
	}
	jsonBytes, err := json.MarshalIndent(v, "", "	")
	if err != nil {
		return "error in json dump :: " + err.Error()
	}
	return string(jsonBytes)
}
