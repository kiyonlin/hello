package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"math/rand"
	"strings"
	"time"
)

// fcoin:// 下單返回1016 資金不足// 下单返回1002 系统繁忙// 返回426 調用次數太頻繁
// coinpark://4003 调用次数繁忙 //2085 最小下单数量限制 //2027 可用余额不足
var bidAskTimes int64
var processing = false
var handling = false
var RefreshCarryChannel = make(chan model.Carry, 50)

func placeRefreshOrder(carry *model.Carry, market, orderSide, orderType string, price, amount float64) {
	if orderSide == `buy` {
		order := api.PlaceOrder(orderSide, orderType, market, carry.BidSymbol, ``, price, amount)
		carry.DealBidOrderId, carry.DealBidErrCode, carry.DealBidAmount, carry.DealBidPrice =
			order.OrderId, order.ErrCode, order.DealAmount, order.DealPrice
		if carry.DealBidOrderId != `` && carry.DealBidOrderId != "0" {
			carry.DealBidStatus = model.CarryStatusWorking
		} else {
			carry.DealBidStatus = model.CarryStatusFail
		}
		util.Notice(fmt.Sprintf(`====%s==== %s %s 价格: %f 数量: %f 返回 %s %s`,
			orderSide, orderType, carry.BidSymbol, price, amount, carry.DealBidOrderId, carry.DealBidErrCode))
	} else if orderSide == `sell` {
		order := api.PlaceOrder(orderSide, orderType, market, carry.AskSymbol, ``, price, amount)
		carry.DealAskOrderId, carry.DealAskErrCode, carry.DealAskAmount, carry.DealAskPrice =
			order.OrderId, order.ErrCode, order.DealAmount, order.DealPrice
		if carry.DealAskOrderId != `` && carry.DealAskOrderId != "0" {
			carry.DealAskStatus = model.CarryStatusWorking
		} else {
			carry.DealAskStatus = model.CarryStatusFail
		}
		util.Notice(fmt.Sprintf(`====%s==== %s %s 价格: %f 数量: %f 返回 %s %s`,
			orderSide, orderType, carry.AskSymbol, price, amount, carry.DealAskOrderId, carry.DealAskErrCode))
	}
	if carry.DealAskErrCode == `2027` || carry.DealBidErrCode == `2027` {
		go api.RefreshAccount(market)
	}
	RefreshCarryChannel <- *carry
	model.CarryChannel <- *carry
}

func setProcessing(value bool) {
	processing = value
}

//func placeExtraSell(carry *model.Carry) {
//	account := model.AppAccounts.GetAccount(model.Fcoin, `ft`)
//	if account == nil {
//		util.Notice(`[额外卖单-nil account]`)
//	} else {
//		util.Notice(fmt.Sprintf(`[额外卖单]%f - %f`, account.Free, model.AppConfig.FtMax))
//	}
//	if account != nil && account.Free > model.AppConfig.FtMax {
//		pricePrecision := util.GetPrecision(carry.BidPrice)
//		if pricePrecision > api.GetPriceDecimal(model.Fcoin, carry.AskSymbol) {
//			pricePrecision = api.GetPriceDecimal(model.Fcoin, carry.AskSymbol)
//		}
//		price := carry.BidPrice * 0.999
//		amount := carry.Amount * model.AppConfig.SellRate
//		order := api.PlaceOrder(model.OrderSideSell, model.OrderTypeLimit,
//			model.GetMarkets()[0], carry.AskSymbol, ``, price, amount)
//		//orderId, errCode, msg, _, _ := orde
//		util.Notice(fmt.Sprintf(`[额外卖单]%s 价格: %f 数量: %f 返回 %s %s %s`,
//			carry.AskSymbol, price, amount, order.OrderId, order.ErrCode, order.Status))
//	}
//}

var ProcessRefresh = func(market, symbol string) {
	carry, err := model.AppMarkets.NewCarry(symbol)
	if err != nil {
		util.Notice(`can not create carry for ` + symbol)
		return
	}
	if model.AppConfig.Handle != `1` || model.AppConfig.HandleRefresh != `1` || processing || handling {
		return
	}
	setProcessing(true)
	defer setProcessing(false)
	timeOk, _ := carry.CheckWorthCarryTime()
	if !timeOk {
		util.SocketInfo(`get carry not on time` + carry.ToString())
		return
	}
	currencies := strings.Split(carry.AskSymbol, "_")
	leftAccount := model.AppAccounts.GetAccount(carry.AskWeb, currencies[0])
	if leftAccount == nil {
		util.Notice(`nil account ` + carry.AskWeb + currencies[0])
		//go getAccount()
		return
	}
	leftBalance := leftAccount.Free
	rightAccount := model.AppAccounts.GetAccount(carry.BidWeb, currencies[1])
	if rightAccount == nil {
		util.Notice(`nil account ` + carry.BidWeb + currencies[1])
		//go getAccount()
		return
	}
	rightBalance := rightAccount.Free
	//pricePrecision := util.GetPrecision(carry.BidPrice)
	//if pricePrecision > api.GetPriceDecimal(carry.BidWeb, carry.BidSymbol) {
	//	pricePrecision = api.GetPriceDecimal(carry.BidWeb, carry.BidSymbol)
	//}
	if model.AppMarkets.BidAsks[symbol] == nil || model.AppMarkets.BidAsks[symbol][market] == nil ||
		len(model.AppMarkets.BidAsks[symbol][market].Bids) == 0 || len(model.AppMarkets.BidAsks[symbol][market].Asks) == 0 {
		util.Notice(`nil bid-ask price for ` + symbol)
		return
	}
	carry.BidPrice = model.AppMarkets.BidAsks[symbol][market].Bids[0].Price
	carry.AskPrice = model.AppMarkets.BidAsks[symbol][market].Asks[0].Price
	carry.BidAmount = model.AppMarkets.BidAsks[symbol][market].Bids[0].Amount
	carry.AskAmount = model.AppMarkets.BidAsks[symbol][market].Asks[0].Amount
	price := (carry.BidPrice + carry.AskPrice) / 2
	util.Notice(fmt.Sprintf(`[%s] %f - %f`, carry.BidSymbol, leftBalance, rightBalance))
	amount := math.Min(leftBalance, rightBalance/carry.BidPrice) * model.AppConfig.AmountRate
	priceDistance := 0.5 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	if (price-carry.BidPrice) < priceDistance || (carry.AskPrice-price) < priceDistance {
		if carry.AskAmount > carry.BidAmount {
			price = carry.BidPrice
			if carry.BidAmount*100 > amount {
				util.Notice(fmt.Sprintf(`[refresh crash]bid:%f - %f`, carry.BidAmount, amount))
				return
			}
		} else {
			price = carry.AskPrice
			if carry.AskAmount*100 > amount {
				util.Notice(fmt.Sprintf(`[refresh crash]ask:%f - %f`, carry.AskAmount, amount))
				return
			}
		}
	}
	model.AppMarkets.BidAsks[carry.AskSymbol][carry.AskWeb] = nil
	model.AppMarkets.BidAsks[carry.BidSymbol][carry.BidWeb] = nil
	bidAskTimes++
	if bidAskTimes%7 == 0 {
		api.RefreshAccount(market)
		//rebalance(leftAccount, rightAccount, carry)
	} else {
		go placeRefreshOrder(carry, market, `buy`, `limit`, price, amount)
		go placeRefreshOrder(carry, market, `sell`, `limit`, price, amount)
		random := rand.Int63n(10000)
		time.Sleep(time.Millisecond * time.Duration(random+model.AppConfig.OrderWait))
	}
}

func RefreshCarryServe() {
	for true {
		orderCarry := <-RefreshCarryChannel
		util.Notice(fmt.Sprintf(`||||||[bid-ask] [%s %s] [%s %s]`, orderCarry.DealBidOrderId,
			orderCarry.DealAskOrderId, orderCarry.DealBidStatus, orderCarry.DealAskStatus))
		if orderCarry.DealBidStatus == `` || orderCarry.DealAskStatus == `` {
			continue
		}
		handling = true
		if orderCarry.DealAskStatus == model.CarryStatusWorking && orderCarry.DealBidStatus == model.CarryStatusWorking {
			time.Sleep(time.Second * 3)
			go api.MustCancel(orderCarry.AskWeb, orderCarry.AskSymbol, orderCarry.DealAskOrderId, false)
			go api.MustCancel(orderCarry.BidWeb, orderCarry.BidSymbol, orderCarry.DealBidOrderId, false)
			//if model.AppConfig.Env == `dk` {
			//	go placeExtraSell(&orderCarry)
			//}
		} else if orderCarry.DealAskStatus == model.CarryStatusWorking && orderCarry.DealBidStatus == model.CarryStatusFail {
			api.MustCancel(orderCarry.AskWeb, orderCarry.AskSymbol, orderCarry.DealAskOrderId, true)
		} else if orderCarry.DealAskStatus == model.CarryStatusFail && orderCarry.DealBidStatus == model.CarryStatusWorking {
			api.MustCancel(orderCarry.BidWeb, orderCarry.BidSymbol, orderCarry.DealBidOrderId, true)
		}
		handling = false
	}
}
