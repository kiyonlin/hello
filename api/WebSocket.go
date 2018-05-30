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
	util.SocketInfo(`get error ` + err.Error())
}

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

func createServer(markets *model.Markets, marketName string) {
	util.SocketInfo(" create chan for " + marketName)
	var channel chan struct{}
	var err error
	switch marketName {
	case model.Huobi:
		channel, err = WsDepthServeHuobi(markets, ProcessCarry, WSErrHandler)
	case model.OKEX:
		channel, err = WsDepthServeOkex(markets, ProcessCarry, WSErrHandler)
	case model.Binance:
		channel, err = WsDepthServeBinance(markets, ProcessCarry, WSErrHandler)
	}
	if err != nil {
		util.SocketInfo(marketName + ` can not create server ` + err.Error())
	}
	markets.PutChan(marketName, channel)
}

var socketMaintaining = false

func MaintainMarketChan() {
	if socketMaintaining {
		return
	}
	socketMaintaining = true
	for _, marketName := range model.ApplicationConfig.Markets {
		subscribes := model.ApplicationConfig.GetSubscribes(marketName)
		for _, subscribe := range subscribes {
			channel := model.ApplicationMarkets.MarketWS[marketName]
			if channel == nil {
				createServer(model.ApplicationMarkets, marketName)
			} else if model.ApplicationMarkets.RequireChanReset(marketName, subscribe) {
				util.SocketInfo(marketName + " need reset " + subscribe)
				model.ApplicationMarkets.PutChan(marketName, nil)
				channel <- struct{}{}
				close(channel)
				createServer(model.ApplicationMarkets, marketName)
			}
			util.SocketInfo(marketName + " new channel reset done")
			break
		}
	}
	socketMaintaining = false
}

func Maintain() {
	for true {
		go MaintainMarketChan()

		delay, _ := model.ApplicationConfig.GetDelay(model.ApplicationConfig.Symbols[0])
		time.Sleep(time.Millisecond * time.Duration(delay))
	}
}

//for _, symbol := range model.ApplicationConfig.Symbols {
//	currencies := strings.Split(symbol, "_")
//	leftTotalPercentage := model.ApplicationAccounts.CurrencyPercentage[currencies[0]]
//	rightTotalPercentage := model.ApplicationAccounts.CurrencyPercentage[currencies[1]]
//	if leftTotalPercentage == 0 || rightTotalPercentage == 0 {
//		continue
//	}
//	leftMarketPercentage := 0.0
//	rightMarketPercentage := 0.0
//	for _, market := range model.ApplicationConfig.Markets {
//		leftAccount := model.ApplicationAccounts.Data[market][currencies[0]]
//		rightAccount := model.ApplicationAccounts.Data[market][currencies[1]]
//		if leftAccount != nil && leftMarketPercentage < leftAccount.Percentage {
//			leftMarketPercentage = leftAccount.Percentage
//		}
//		if rightAccount != nil && rightMarketPercentage < rightAccount.Percentage {
//			rightMarketPercentage = rightAccount.Percentage
//		}
//	}
//if leftMarketPercentage == 0 || rightMarketPercentage == 0 {
//	continue
//}
//balanceRate := leftTotalPercentage / leftMarketPercentage
//if balanceRate > rightTotalPercentage/rightMarketPercentage {
//	balanceRate = rightTotalPercentage / rightMarketPercentage
//}
//if balanceRate < 0.5 {
//	model.ApplicationConfig.IncreaseMargin(symbol)
//} else {
//	model.ApplicationConfig.DecreaseMargin(symbol)
//}
//margin, _ := model.ApplicationConfig.GetMargin(symbol)
//util.Notice(fmt.Sprintf(`%s margin: %.5f`, symbol, margin))
//}
