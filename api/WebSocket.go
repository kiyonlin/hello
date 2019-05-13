package api

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"hello/model"
	"hello/util"
	"net/http"
)

type MsgHandler func(message []byte)
type SubscribeHandler func(subscribes []interface{}, subType string) error
type ErrHandler func(err error)

func sendToWs(market string, msg []byte) (err error) {
	if model.AppMarkets.GetIsWriting(market) {
		return errors.New(fmt.Sprintf(`conn %s is writing`, market))
	}
	model.AppMarkets.SetIsWriting(market, true)
	defer model.AppMarkets.SetIsWriting(market, false)
	conn := model.AppMarkets.GetConn(market)
	if conn == nil {
		return errors.New(fmt.Sprintf(`conn %s is nil`, market))
	}
	return conn.WriteMessage(websocket.TextMessage, msg)
}

func newConnection(url string) (*websocket.Conn, error) {
	var connErr error
	var c *websocket.Conn
	//for i := 0; i < 10; i++ {
	util.SocketInfo("try to connect " + url)
	dialer := &websocket.Dialer{
		Proxy: http.ProxyFromEnvironment,
	}
	c, _, connErr = dialer.Dial(url, nil)
	if connErr == nil {
		//	break
	} else {
		util.SocketInfo(`can not create new connection ` + connErr.Error())
		if c != nil {
			_ = c.Close()
		}
	}
	//	time.Sleep(1000)
	//}
	if connErr != nil {
		return nil, connErr
	}
	return c, nil
}

func chanHandler(market string, stopC chan struct{}, errHandler ErrHandler, msgHandler MsgHandler) {
	conn := model.AppMarkets.GetConn(market)
	defer func() {
		err := conn.Close()
		if err != nil {
			errHandler(err)
		}
	}()
	for true {
		select {
		case <-stopC:
			util.Notice("get stop struct, return")
			return
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				util.Notice("can not read from websocket: " + err.Error())
				return
			}
			//util.SocketInfo(string(message))
			msgHandler(message)
		}
	}
}

func WebSocketServe(market, url, subType string, subscribes []interface{}, subHandler SubscribeHandler,
	msgHandler MsgHandler, errHandler ErrHandler) (chan struct{}, error) {
	conn, err := newConnection(url)
	if err != nil {
		util.SocketInfo("can not create web socket" + err.Error())
		errHandler(err)
		return nil, err
	}
	model.AppMarkets.SetConn(market, conn)
	_ = subHandler(subscribes, subType)
	stopC := make(chan struct{}, 10)
	go chanHandler(market, stopC, errHandler, msgHandler)
	return stopC, err
}
