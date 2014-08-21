package main

import (
	"fmt"
	"os"

	"github.com/bom-d-van/xmppclient"
)

func main() {
	conn, err := xmppclient.Dial(
		"localhost:5222",
		//"yeerkunth-theplant@localhost",
		"y21@localhost",
		"localhost",
		//"BPwOQnLGnJ",
		"nopassword",
		"", //let server generate the resource
		&xmppclient.Config{
			Log: os.Stderr,
			// Log:    logutils.ToWriter(logger),
			// InLog:  logutils.ToWriter(in),
			// OutLog: logutils.ToWriter(out),
		},
	)

	if err != nil {
		panic(err)
	}

	// conn.Presence = make(chan *xmppclient.ClientPresence)
	// conn.Message = make(chan *xmppclient.ClientMessage)
	conn.SignalPresence("1")
	//conn.Send("enn.raven-theplant@localhost", "I came from the darkness")
	conn.Send("y21@localhost", "I came from the darkness")

	//conn.JoinMUC("49qniykfbt9@conference.localhost", "y")
	//conn.SendGroupChatMessage("49qniykfbt9@conference.localhost", "I came from the darkness")

	//conn.SendMediatedMucInvitation("enn.raven-theplant@localhost", "49qniykfbt9@conference.localhost", "noreason")
	//conn.JoinMUC("bullshit@conference.localhost", "y")
	//conn.SendMediatedMucInvitation("enn.raven-theplant@localhost", "bullshit@conference.localhost", "noreason")
	//conn.SendDirectMucInvitation("enn.raven-theplant@localhost", "bullshit@conference.localhost", "noreason")

	conn.Handler = &xmppclient.BasicHandler{}
	conn.Listen()
	fmt.Println("hh")

	// conn.Send("jiangnan34-theplant@localhost", "it's my message.")
	// conn.DiscoverRooms()

	// for {
	// 	msg := <-conn.Message
	// 	log.Printf("--> %+v\n", msg.Body)
	// }
}
