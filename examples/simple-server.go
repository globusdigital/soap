package main

import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"

	"github.com/globusdigital/soap"
)

// FooRequest a simple request
type FooRequest struct {
	XMLName xml.Name `xml:"fooRequest"`
	Foo     string
}

// FooResponse a simple response
type FooResponse struct {
	Bar string
}

// RunServer run a little demo server
func RunServer() {
	soapServer := soap.NewServer()
	soapServer.Log = log.Println
	soapServer.RegisterHandler(
		"/pathTo",
		"operationFoo", // SOAPAction
		"fooRequest",   // tagname of soap body content
		// RequestFactoryFunc - give the server sth. to unmarshal the request into
		func() any {
			return &FooRequest{}
		},
		// OperationHandlerFunc - do something
		func(request any, w http.ResponseWriter, httpRequest *http.Request) (response any, err error) {
			fooRequest := request.(*FooRequest)
			fooResponse := &FooResponse{
				Bar: "Hello \"" + fooRequest.Foo + "\"",
			}
			response = fooResponse
			return
		},
	)
	err := http.ListenAndServe(":8080", soapServer)
	fmt.Println("exiting with error", err)
}

func main() {
	RunServer()
}
