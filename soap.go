package soap

import (
	"encoding/xml"
)

// SOAP 1.1 and SOAP 1.2 must expect different ContentTypes and Namespaces.

const (
	Version11 = "1.1"
	Version12 = "1.2"

	ContentType11 = "text/xml; charset=\"utf-8\""
	ContentType12 = "application/soap+xml; charset=\"utf-8\""

	NamespaceSoap11 = "http://schemas.xmlsoap.org/soap/envelope/"
	NamespaceSoap12 = "http://www.w3.org/2003/05/soap-envelope"
)

var (
	bNamespaceSoap11 = []byte(NamespaceSoap11)
	bNamespaceSoap12 = []byte(NamespaceSoap12)
)

// Envelope type `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
type Envelope struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	Header  Header
	Body    Body
}

// Header type
type Header struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Header"`

	Header any
}

// Body type
type Body struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`

	Fault               *Fault `xml:",omitempty"`
	Content             any    `xml:",omitempty"`
	SOAPBodyContentType string `xml:"-"`
}

// Fault type
type Fault struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault"`

	Code   string `xml:"faultcode,omitempty"`
	String string `xml:"faultstring,omitempty"`
	Actor  string `xml:"faultactor,omitempty"`
	Detail string `xml:"detail,omitempty"`
}

// UnmarshalXML implement xml.Unmarshaler
func (b *Body) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	if b.Content == nil {
		return xml.UnmarshalError("Content must be a pointer to a struct")
	}

	var (
		token    xml.Token
		err      error
		consumed bool
	)

Loop:
	for {
		if token, err = d.Token(); err != nil {
			return err
		}

		if token == nil {
			break
		}

		switch se := token.(type) {
		case xml.StartElement:
			if consumed {
				return xml.UnmarshalError("Found multiple elements inside SOAP body; not wrapped-document/literal WS-I compliant")
			} else if se.Name.Space == "http://schemas.xmlsoap.org/soap/envelope/" && se.Name.Local == "Fault" {
				b.Fault = &Fault{}
				b.Content = nil

				err = d.DecodeElement(b.Fault, &se)
				if err != nil {
					return err
				}

				consumed = true
			} else {
				b.SOAPBodyContentType = se.Name.Local
				if err = d.DecodeElement(b.Content, &se); err != nil {
					return err
				}

				consumed = true
			}
		case xml.EndElement:
			break Loop
		}
	}

	return nil
}

func (f *Fault) Error() string {
	return f.String
}
