# qbit

[![GoDoc](https://godoc.org/golift.io/qbit?status.svg)](https://pkg.go.dev/golift.io/qbit)
[![Go Report Card](https://goreportcard.com/badge/golift.io/qbit)](https://goreportcard.com/report/golift.io/qbit)
[![MIT License](http://img.shields.io/:license-mit-blue.svg)](https://github.com/golift/qbit/blob/main/LICENSE)

Go library to interact with the [qBittorrent](https://github.com/qbittorrent/qBittorrent) Web API.

This is not a full client. It covers login (including HTTP basic auth), listing transfers,
listing categories, and setting a torrent category. If you need more of the API, please
[open an issue](https://github.com/golift/qbit/issues/new) or a pull request.

This library is used by [Notifiarr](https://github.com/Notifiarr/notifiarr/).

qBittorrent 5.2+ returns HTTP 204 on login and other empty WebAPI responses.
Older 200/`Ok.` logins still work.

## Install

```shell
go get golift.io/qbit
```

## Example

```go
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"golift.io/qbit"
)

func main() {
	q, err := qbit.New(context.Background(), &qbit.Config{
		URL:  "http://localhost:8080",
		User: "admin",
		Pass: "qbitpassword",
		// Optional HTTP basic auth in front of the Web UI:
		// HTTPUser: "nginx",
		// HTTPPass: "secret",
		Client: &http.Client{Timeout: time.Minute},
	})
	if err != nil {
		log.Fatalln("[ERROR]", err)
	}

	xfers, err := q.GetXfers()
	if err != nil {
		log.Fatalln("[ERROR]", err)
	}

	for _, xfer := range xfers {
		log.Println(xfer.Name, xfer.Progress, xfer.SavePath)
	}
}
```

`New` logs in immediately. `NewNoAuth` skips that and logs in on the first API call
(useful when you only need the client constructed at startup).
