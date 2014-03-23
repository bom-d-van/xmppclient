package xmppclient

type Handler interface {
	RecvMsg(msg *ClientMessage)
	RecvPresence(pres *ClientPresence)
}

type BasicHandler struct{}

func (b *BasicHandler) RecvMsg(msg *ClientMessage)        {}
func (b *BasicHandler) RecvPresence(pres *ClientPresence) {}
