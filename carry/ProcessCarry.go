package carry

import (
	"fmt"
	"github.com/jinzhu/configor"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"hello/api"
	"hello/model"
	"hello/util"
	"strconv"
	"strings"
	"time"
)

var WSErrHandler = func(err error) {
	print(err)
	util.SocketInfo(`get error ` + err.Error())
}

// 只取第一位
func calcAmount(originalAmount float64) (num float64, err error) {
	str := strconv.FormatFloat(originalAmount, 'f', -1, 64)
	bytes := []byte(str)
	startReplace := false
	for i, v := range bytes {
		if startReplace && v != '.' {
			bytes[i] = '0'
		}
		if v != '0' && v != '.' {
			startReplace = true
		}
	}
	return strconv.ParseFloat(string(bytes), 64)
}

var ProcessCarry = func(carry *model.Carry) {
	currencies := strings.Split(carry.Symbol, "_")
	leftBalance := 0.0
	rightBalance := 0.0
	account := model.ApplicationAccounts.GetAccount(carry.AskWeb, currencies[0])
	if account == nil {
		util.Info(`nil account ` + carry.AskWeb + currencies[0])
		return
	}
	leftBalance = account.Free
	account = model.ApplicationAccounts.GetAccount(carry.BidWeb, currencies[1])
	if account == nil {
		util.Info(`nil account ` + carry.BidWeb + currencies[1])
		return
	}
	rightBalance = account.Free
	priceInUsdt, _ := model.GetBuyPriceOkex(currencies[0] + "_usdt")
	minAmount := model.ApplicationConfig.MinUsdt / priceInUsdt
	maxAmount := model.ApplicationConfig.MaxUsdt / priceInUsdt
	if carry.Amount > maxAmount {
		carry.Amount = maxAmount
	}
	if leftBalance > carry.Amount {
		leftBalance = carry.Amount
	}
	if leftBalance*carry.BidPrice > rightBalance {
		leftBalance = rightBalance / carry.BidPrice
	}
	planAmount, _ := calcAmount(carry.Amount)
	carry.Amount = planAmount
	leftBalance, _ = calcAmount(leftBalance)
	strLeftBalance := strconv.FormatFloat(leftBalance, 'f', -1, 64)
	strAskPrice := strconv.FormatFloat(carry.AskPrice, 'f', -1, 64)
	strBidPrice := strconv.FormatFloat(carry.BidPrice, 'f', -1, 64)

	timeOk, _ := carry.CheckWorthCarryTime(model.ApplicationMarkets, model.ApplicationConfig)
	marginOk, _ := carry.CheckWorthCarryMargin(model.ApplicationMarkets, model.ApplicationConfig)
	if !carry.CheckWorthSaveMargin() {
		// no need to save carry with margin < base cost
		return
	}
	doCarry := false
	if !timeOk {
		carry.DealAskStatus = `NotOnTime`
		carry.DealBidStatus = `NotOnTime`
		util.SocketInfo(`get carry not on time` + carry.ToString())
	} else {
		if !marginOk {
			carry.DealAskStatus = `NotWorth`
			carry.DealBidStatus = `NotWorth`
			util.SocketInfo(`get carry no worth` + carry.ToString())
		} else {
			model.ApplicationMarkets.BidAsks[carry.Symbol][carry.AskWeb] = nil
			model.ApplicationMarkets.BidAsks[carry.Symbol][carry.BidWeb] = nil
			if leftBalance < minAmount {
				carry.DealAskStatus = `NotEnough`
				carry.DealBidStatus = `NotEnough`
				util.Info(fmt.Sprintf(`leftB %f rightB/bidPrice %f/%f NotEnough %f - %f %s`, account.Free,
					rightBalance, carry.BidPrice, leftBalance, minAmount, carry.ToString()))
			} else {
				if model.ApplicationConfig.Env == `test` {
					carry.DealAskStatus = `NotDo`
					carry.DealBidStatus = `NotDo`
				} else {
					util.Notice(`get carry worth` + carry.ToString())
					doCarry = true
				}
			}
		}
	}
	if doCarry {
		go api.DoAsk(carry, strAskPrice, strLeftBalance)
		go api.DoBid(carry, strBidPrice, strLeftBalance)
	} else {
		model.BidChannel <- *carry
	}
}

func createServer(markets *model.Markets, carryHandler api.CarryHandler, marketName string) chan struct{} {
	util.SocketInfo(" create chan for " + marketName)
	var channel chan struct{}
	var err error
	switch marketName {
	case model.Huobi:
		channel, err = api.WsDepthServeHuobi(markets, carryHandler, WSErrHandler)
	case model.OKEX:
		channel, err = api.WsDepthServeOkex(markets, carryHandler, WSErrHandler)
	case model.Binance:
		channel, err = api.WsDepthServeBinance(markets, carryHandler, WSErrHandler)
	case model.Fcoin:
		channel, err = api.WsDepthServeFcoin(markets, carryHandler, WSErrHandler)
	}
	if err != nil {
		util.SocketInfo(marketName + ` can not create server ` + err.Error())
	}
	return channel
}

var socketMaintaining = false

func MaintainMarketChan(carryHandler api.CarryHandler) {
	if socketMaintaining {
		return
	}
	socketMaintaining = true
	for _, marketName := range model.ApplicationConfig.Markets {
		subscribes := model.ApplicationConfig.GetSubscribes(marketName)
		for _, subscribe := range subscribes {
			for index := 0; index < model.ApplicationConfig.Channels; index++ {
				channel := model.ApplicationMarkets.GetChan(marketName, index)
				if channel == nil {
					model.ApplicationMarkets.PutChan(marketName, index, createServer(model.ApplicationMarkets,
						carryHandler, marketName))
				} else if model.ApplicationMarkets.RequireChanReset(marketName, subscribe) {
					//util.SocketInfo(marketName + " need reset " + subscribe)
					model.ApplicationMarkets.PutChan(marketName, index, nil)
					channel <- struct{}{}
					close(channel)
					model.ApplicationMarkets.PutChan(marketName, index, createServer(model.ApplicationMarkets,
						carryHandler, marketName))
				}
				//util.SocketInfo(marketName + " new channel reset done")
			}
			break
		}
	}
	socketMaintaining = false
}

func Maintain() {
	util.Notice("start carrying")
	model.NewConfig()
	err := configor.Load(model.ApplicationConfig, "./config.yml")
	if err != nil {
		print(err)
		return
	}
	model.SetApiKeys()
	model.ApplicationDB, err = gorm.Open("postgres", model.ApplicationConfig.DBConnection)
	if err != nil {
		util.Notice(err.Error())
		return
	}
	defer model.ApplicationDB.Close()
	model.ApplicationDB.AutoMigrate(&model.Carry{})
	model.ApplicationDB.AutoMigrate(&model.Account{})
	go api.RefreshAccounts()
	go DBHandlerServe()
	go AskUpdate()
	go BidUpdate()
	go AccountDBHandlerServe()
	go MaintainOrders()
	model.ApplicationMarkets = model.NewMarkets()
	for true {
		go MaintainMarketChan(ProcessCarry)
		time.Sleep(time.Duration(model.ApplicationConfig.ChannelSlot) * time.Millisecond)
	}
}
