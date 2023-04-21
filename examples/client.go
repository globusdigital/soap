package main

import (
	"context"
	"encoding/xml"
	"log"

	soap "github.com/daliand/gdsoap"
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

func main() {
	client := soap.NewClient("http://127.0.0.1:8080/", nil)
	client.Log = func(msg string, keyString_ValueInterface ...interface{}) {
		keyString_ValueInterface = append(keyString_ValueInterface, msg)
		log.Println(keyString_ValueInterface...)
	} // verbose
	response := &FooResponse{}
	httpResponse, err := client.Call(context.Background(), "operationFoo", &FooRequest{Foo: "hello i am foo"}, response)
	if err != nil {
		panic(err)
	}
	log.Println(response.Bar, httpResponse.Status)
}
