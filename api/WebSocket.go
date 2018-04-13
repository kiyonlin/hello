package api

import (
	"github.com/gorilla/websocket"
	"time"
	"hello/util"
	"hello/model"
)

// WsHandler handle raw websocket message
type MsgHandler func(message []byte, conn *websocket.Conn)
type SubscribeHandler func(subscribes []string, conn *websocket.Conn) error
type ErrHandler func(err error)

var WSErrHandler = func(err error) {
	print(err)
	util.SocketInfo("DO NOTHING")
}

func newConnection(url string) (*websocket.Conn, error) {
	var connErr error
	var c *websocket.Conn
	for i := 0; i < 10; i++ {
		util.SocketInfo("try to connect " + url)
		c, _, connErr = websocket.DefaultDialer.Dial(url, nil)
		if connErr == nil {
			break
		} else {
			print(connErr)
			if c != nil {
				c.Close()
			}
		}
		time.Sleep(5000)
	}
	if connErr != nil {
		return nil, connErr
	}
	return c, nil
}

func chanHandler(c *websocket.Conn, stopC chan struct{}, errHandler ErrHandler, msgHandler MsgHandler) {
	defer func() {
		close(stopC)
		err := c.Close()
		if err != nil {
			errHandler(err)
		}
	}()
	for {
		select {
		case <-stopC:
			util.Info("get stop struct, return")
			return
		default:
			_, message, err := c.ReadMessage()
			if err != nil {
				util.SocketInfo("can not read from websocket: " + err.Error())
			}
			msgHandler(message, c)
		}
	}
}

func WebSocketServe(url string, subscribes []string, subHandler SubscribeHandler, msgHandler MsgHandler, errHandler ErrHandler) (chan struct{}, error) {
	c, err := newConnection(url)
	if err != nil {
		errHandler(err)
		return nil, err
	}
	subHandler(subscribes, c)
	stopC := make(chan struct{})
	go chanHandler(c, stopC, errHandler, msgHandler)
	return stopC, err
}

func createServer(markets *model.Markets, marketName string) {
	util.SocketInfo(" create chan for " + marketName)
	var channel chan struct{}
	switch marketName {
	case model.Huobi:
		channel, _ = WsDepthServeHuobi(markets, ProcessCarry, WSErrHandler)
	case model.OKEX:
		channel, _ = WsDepthServeOkex(markets, ProcessCarry, WSErrHandler)
	case model.Binance:
		channel, _ = WsDepthServeBinance(markets, ProcessCarry, WSErrHandler)
	}
	markets.PutChan(marketName, channel)
}

func maintainMarketChan(markets *model.Markets, marketName string, subscribe string) {
	channel := markets.GetChan(marketName)
	if channel == nil {
		createServer(markets, marketName)
	} else if markets.RequireChanReset(marketName, subscribe) {
		util.SocketInfo(marketName + " need reset " + subscribe)
		channel <- struct{}{}
		createServer(markets, marketName)
	}
}

func Maintain(markets *model.Markets, config *model.Config) {
	for true {
		for _, marketName := range config.Markets {
			subscribes := config.GetSubscribes(marketName)
			for _, subscribe := range subscribes {
				go maintainMarketChan(markets, marketName, subscribe)
				break
			}
		}
		time.Sleep(time.Minute * 1)
	}
}
