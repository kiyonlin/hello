package api

import (
	"github.com/gorilla/websocket"
	"time"
	"hello/util"
	"hello/model"
	"strings"
	"fmt"
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
			msgHandler(message, c)
		}
	}
}

func WebSocketServe(url string, subscribes []string, subHandler SubscribeHandler, msgHandler MsgHandler, errHandler ErrHandler) (chan struct{}, error) {
	c, err := newConnection(url)
	if err != nil {
		util.SocketInfo("can not create web socket")
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
		createServer(markets, marketName)
		channel <- struct{}{}
		close(channel)
		markets.PutChan(marketName, nil)
		util.SocketInfo(marketName + " channel closed and cleared ")
	}
}

func Maintain(markets *model.Markets, config *model.Config) {
	for true {
		for _, marketName := range config.Markets {
			subscribes := config.GetSubscribes(marketName)
			for _, subscribe := range subscribes {
				maintainMarketChan(markets, marketName, subscribe)
				break
			}
		}
		for _, symbol := range model.ApplicationConfig.Symbols {
			currencies := strings.Split(symbol, "_")
			leftTotalPercentage := model.ApplicationAccounts.CurrencyPercentage[currencies[0]]
			rightTotalPercentage := model.ApplicationAccounts.CurrencyPercentage[currencies[1]]
			if leftTotalPercentage == 0 || rightTotalPercentage == 0 {
				continue
			}
			leftMarketPercentage := 0.0
			rightMarketPercentage := 0.0
			for _, market := range model.ApplicationConfig.Markets {
				leftAccount := model.ApplicationAccounts.Data[market][currencies[0]]
				rightAccount := model.ApplicationAccounts.Data[market][currencies[1]]
				if leftAccount != nil && leftMarketPercentage < leftAccount.Percentage {
					leftMarketPercentage = leftAccount.Percentage
				}
				if rightAccount != nil && rightMarketPercentage < rightAccount.Percentage {
					rightMarketPercentage = rightAccount.Percentage
				}
			}
			if leftMarketPercentage == 0 || rightMarketPercentage == 0 {
				continue
			}
			//balanceRate := leftTotalPercentage / leftMarketPercentage
			//if balanceRate > rightTotalPercentage/rightMarketPercentage {
			//	balanceRate = rightTotalPercentage / rightMarketPercentage
			//}
			//if balanceRate < 0.5 {
			//	model.ApplicationConfig.IncreaseMargin(symbol)
			//} else {
			//	model.ApplicationConfig.DecreaseMargin(symbol)
			//}
			margin, _ := model.ApplicationConfig.GetMargin(symbol)
			util.SocketInfo(fmt.Sprintf(`%s margin: %.5f`, symbol, margin))
		}
		time.Sleep(time.Minute * 2)
	}
}
