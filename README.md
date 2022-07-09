# Go Library: qbit

Go Library used to interact with Qbittorrent. This library is not full-featured,
and probably only provides read-only methods to view the download list.

At the time of this writing (creation), the library only supports logging in,
and collecting transfer info.

If you'd like new features, please open a GitHub issue or pull request.

Example:

```go
package main

import (
	"log"
	"time"

	"golift.io/qbit"
)

func main() {
	config := &qbit.Config{
		URL:     "http://localhost:8080",
		User:    "admin",
		Pass:    "qbitpassword",
	}

	qbit, err := qbit.New(config)
	if err != nil {
		log.Fatalln("[ERROR]", err)
	}

	xfers, err := qbit.GetXfers()
	if err != nil {
		log.Fatalln("[ERROR]", err)
	}

	for _, xfer := range xfers {
		log.Println(xfer.Name, xfer.Progress)
	}
}
```
