package xmppclient

import "fmt"

type Handler interface {
	RecvMsg(msg *ClientMessage)
	RecvPresence(pres *ClientPresence)
}

type BasicHandler struct{}

func (b *BasicHandler) RecvMsg(msg *ClientMessage) {
	fmt.Println(msg)
	return
}

func (b *BasicHandler) RecvPresence(pres *ClientPresence) {
	fmt.Println(pres)
	return
}
