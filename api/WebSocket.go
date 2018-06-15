package api

import (
	"github.com/gorilla/websocket"
	"hello/model"
	"hello/util"
)

type MsgHandler func(message []byte, conn *websocket.Conn)
type SubscribeHandler func(subscribes []string, conn *websocket.Conn) error
type ErrHandler func(err error)
type CarryHandler func(carry *model.Carry)

func newConnection(url string) (*websocket.Conn, error) {
	var connErr error
	var c *websocket.Conn
	//for i := 0; i < 10; i++ {
	util.SocketInfo("try to connect " + url)
	c, _, connErr = websocket.DefaultDialer.Dial(url, nil)
	if connErr == nil {
		//	break
	} else {
		util.SocketInfo(`can not create new connection ` + connErr.Error())
		if c != nil {
			c.Close()
		}
	}
	//	time.Sleep(1000)
	//}
	if connErr != nil {
		return nil, connErr
	}
	return c, nil
}

func chanHandler(c *websocket.Conn, stopC chan struct{}, errHandler ErrHandler, msgHandler MsgHandler) {
	defer func() {
		err := c.Close()
		if err != nil {
			errHandler(err)
		}
	}()
	for true {
		select {
		case <-stopC:
			util.SocketInfo("get stop struct, return")
			return
		default:
			_, message, err := c.ReadMessage()
			if err != nil {
				util.SocketInfo("can not read from websocket: " + err.Error())
				return
			}
			//util.SocketInfo(string(message))
			msgHandler(message, c)
		}
	}
}

func WebSocketServe(url string, subscribes []string, subHandler SubscribeHandler, msgHandler MsgHandler,
	errHandler ErrHandler) (chan struct{}, error) {
	c, err := newConnection(url)
	if err != nil {
		util.SocketInfo("can not create web socket")
		errHandler(err)
		return nil, err
	}
	subHandler(subscribes, c)
	stopC := make(chan struct{}, 10)
	go chanHandler(c, stopC, errHandler, msgHandler)
	return stopC, err
}
