package main

import (
	"os"

	"github.com/bom-d-van/xmppclient"
)

func main() {
	conn, err := xmppclient.Dial("localhost:5222", "bom.d.van-theplant@localhost", "localhost", "JoeQHcjt9z", &xmppclient.Config{
		Log: os.Stderr,
		// Log:    logutils.ToWriter(logger),
		// InLog:  logutils.ToWriter(in),
		// OutLog: logutils.ToWriter(out),
	})

	if err != nil {
		panic(err)
	}

	// conn.Presence = make(chan *xmppclient.ClientPresence)
	// conn.Message = make(chan *xmppclient.ClientMessage)
	conn.SignalPresence("1")
	conn.Listen()

	// conn.Send("jiangnan34-theplant@localhost", "it's my message.")
	// conn.DiscoverRooms()

	// for {
	// 	msg := <-conn.Message
	// 	log.Printf("--> %+v\n", msg.Body)
	// }
}
