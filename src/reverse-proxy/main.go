package main

import (
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime"
	"sync"
	"time"
)

// Upstream is used to load balance the incoming connections
type Upstream struct {
	addresses  []*string
	nAddresses uint32
	nextConn   uint32
}

func (upstream *Upstream) addAddress(address string) {
	upstream.addresses = append(upstream.addresses, &address)
	upstream.nAddresses++
}

func (upstream *Upstream) getAddress() *string {
	address := upstream.addresses[upstream.nextConn]

	upstream.nextConn++
	if upstream.nextConn == upstream.nAddresses {
		upstream.nextConn = 0
	}

	return address
}

func main() {
	runtime.GOMAXPROCS(1)

	upstream := Upstream{}
	upstream.addAddress("127.0.0.1:3000")
	upstream.addAddress("127.0.0.1:3001")

	http.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		// Dumps the request to pipe it to destination server using raw TCP
		requestDump, err := httputil.DumpRequest(req, true)
		if err != nil {
			res.WriteHeader(http.StatusInternalServerError)
			return
		}

		destConn, resConn := net.Pipe()

		// Gets the destination server address
		destAddress := upstream.getAddress()
		// Dials the destination server
		destConn, err = net.DialTimeout("tcp", *destAddress, time.Second*5)
		if err != nil {
			res.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer destConn.Close()

		// Writes the dumped request to the destination server
		_, err = destConn.Write(requestDump)
		if err != nil {
			res.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Hijacks the ResponseWriter to pipe the raw response from the destination server
		hj, ok := res.(http.Hijacker)
		if !ok {
			res.WriteHeader(http.StatusInternalServerError)
			return
		}
		resConn, _, err = hj.Hijack()
		if err != nil {
			return
		}
		defer resConn.Close()

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			// Pipes client's response to the destination server
			buf := make([]byte, 1024)
			for {
				n, err2 := resConn.Read(buf)
				if err2 != nil {
					destConn.Close()
					break
				}
				destConn.Write(buf[:n])
			}
		}()

		go func() {
			defer wg.Done()
			// Pipes destination's response to the client
			buf := make([]byte, 1024)
			for {
				n, err3 := destConn.Read(buf)
				if err3 != nil {
					resConn.Close()
					break
				}
				resConn.Write(buf[:n])
			}
		}()

		wg.Wait()
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
