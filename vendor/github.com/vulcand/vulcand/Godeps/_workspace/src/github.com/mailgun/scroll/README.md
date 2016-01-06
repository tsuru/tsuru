# scroll

[![Build Status](http://img.shields.io/travis/mailgun/scroll/master.svg)](https://travis-ci.org/mailgun/scroll)


Scroll is a lightweight library for building Go HTTP services at Mailgun. It is
built on top of [mux](http://www.gorillatoolkit.org/pkg/mux) and adds:

- Service Discovery
- Graceful Shutdown
- Configurable Logging
- Request Metrics

**Scroll is a work in progress. Use at your own risk.**

## Installation

```
go get github.com/mailgun/scroll
```

## Getting Started

Building an application with Scroll is simple. Here's a server that listens for GET or POST requests to `http://0.0.0.0:8080/resources/{resourceID}` and echoes back the resource ID provided in the URL.

```go
package main

import (
	"fmt"
	"net/http"

	"github.com/mailgun/scroll"
	"github.com/mailgun/scroll/registry"
)

func handler(w http.ResponseWriter, r *http.Request, params map[string]string) (interface{}, error) {
	return scroll.Response{
		"message": fmt.Sprintf("Resource ID: %v", params["resourceID"]),
	}, nil
}

func main() {
	// create an app
	appConfig := scroll.AppConfig{
		Name:       "scrollexample",
		ListenIP:   "0.0.0.0",
		ListenPort: 8080,
		Registry:   &registry.NopRegistry{},
	}
	app := scroll.NewAppWithConfig(appConfig)

	// register a handler
	handlerSpec := scroll.Spec{
		Methods:  []string{"GET", "POST"},
		Paths:    []string{"/resources/{resourceID}"},
		Handler:  handler,
	}

	app.AddHandler(handlerSpec)

	// start the app
	app.Run()
}
```
