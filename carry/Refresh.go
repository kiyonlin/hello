package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"strconv"
	"strings"
	"time"
)

// fcoin:// 下單返回1016 資金不足// 下单返回1002 系统繁忙// 返回426 調用次數太頻繁
// coinpark://4003 调用次数繁忙 //2085 最小下单数量限制 //2027 可用余额不足
var lastOrderTime int64
var bidAskTimes int64
var processing = false
var handling = false

func calcPrice(bidPrice, askPrice float64, precision int) (num float64, err error) {
	str := strconv.FormatFloat(bidPrice+(askPrice-bidPrice)*1/2, 'f', precision, 64)
	return strconv.ParseFloat(str, 64)
}

func getLeftRightAmounts(leftBalance, rightBalance float64, carry *model.Carry) (askAmount, bidAmount float64) {
	amountPrecision := util.GetPrecision(carry.BidAmount)
	if model.AppConfig.Env == `dk` {
		if leftBalance*model.AppConfig.AmountRate > rightBalance/carry.BidPrice {
			carry.Amount = rightBalance / carry.BidPrice
		} else {
			carry.Amount = leftBalance * model.AppConfig.AmountRate
		}
		askAmount = carry.Amount
		bidAmount = carry.Amount
	} else {
		askAmount = leftBalance * model.AppConfig.AmountRate
		bidAmount = rightBalance * model.AppConfig.AmountRate / carry.BidPrice
	}
	strAskAmount := strconv.FormatFloat(askAmount, 'f', amountPrecision, 64)
	strBidAmount := strconv.FormatFloat(bidAmount, 'f', amountPrecision, 64)
	askAmount, _ = strconv.ParseFloat(strAskAmount, 64)
	bidAmount, _ = strconv.ParseFloat(strBidAmount, 64)
	return askAmount, bidAmount
}

func placeRefreshOrder(carry *model.Carry, orderSide, orderType string, price, amount float64) {
	if orderSide == `buy` {
		carry.DealBidOrderId, carry.DealBidErrCode, _, carry.BidAmount, carry.BidPrice =
			api.PlaceOrder(orderSide, orderType, model.GetMarkets()[0], carry.Symbol, ``, price, amount)
		if carry.DealBidOrderId != `` && carry.DealBidOrderId != "0" {
			carry.DealBidStatus = model.CarryStatusWorking
		} else {
			carry.DealBidStatus = model.CarryStatusFail
		}
		util.Notice(fmt.Sprintf(`====%s==== %s %s 价格: %s 数量: %s 返回 %s %s`,
			orderSide, orderType, carry.Symbol, price, amount, carry.DealBidOrderId, carry.DealBidErrCode))
	} else if orderSide == `sell` {
		carry.DealAskOrderId, carry.DealAskErrCode, _, carry.AskAmount, carry.AskPrice =
			api.PlaceOrder(orderSide, orderType, carry.Symbol, orderType, ``, price, amount)
		if carry.DealAskOrderId != `` && carry.DealAskOrderId != "0" {
			carry.DealAskStatus = model.CarryStatusWorking
		} else {
			carry.DealAskStatus = model.CarryStatusFail
		}
		util.Notice(fmt.Sprintf(`====%s==== %s %s 价格: %s 数量: %s 返回 %s %s`,
			orderSide, orderType, carry.Symbol, price, amount, carry.DealAskOrderId, carry.DealAskErrCode))
	}
	if carry.DealAskErrCode == `2027` || carry.DealBidErrCode == `2027` {
		go api.RefreshAccount(model.GetMarkets()[0])
	}
	model.RefreshCarryChannel <- *carry
	model.CarryChannel <- *carry
}

func setProcessing(value bool) {
	processing = value
}

func placeExtraSell(carry *model.Carry) {
	account := model.AppAccounts.GetAccount(model.Fcoin, `ft`)
	if account == nil {
		util.Notice(`[额外卖单-nil account]`)
	} else {
		util.Notice(fmt.Sprintf(`[额外卖单]%f - %f`, account.Free, model.AppConfig.FtMax))
	}
	if account != nil && account.Free > model.AppConfig.FtMax {
		pricePrecision := util.GetPrecision(carry.BidPrice)
		if pricePrecision > api.GetPriceDecimal(model.Fcoin, carry.Symbol) {
			pricePrecision = api.GetPriceDecimal(model.Fcoin, carry.Symbol)
		}
		price := carry.BidPrice * 0.999
		amount := carry.Amount * model.AppConfig.SellRate
		orderId, errCode, msg, _, _ := api.PlaceOrder(model.OrderSideSell, model.OrderTypeLimit,
			model.GetMarkets()[0], carry.Symbol, ``, price, amount)
		util.Notice(fmt.Sprintf(`[额外卖单]%s 价格: %f 数量: %f 返回 %s %s %s`,
			carry.Symbol, price, amount, orderId, errCode, msg))
	}
}

var ProcessRefresh = func(symbol, market string) {
	carry, err := model.AppMarkets.NewCarry(symbol)
	if err != nil {
		util.Notice(`can not create carry for ` + symbol)
		return
	}
	if model.AppConfig.Handle == 0 || processing || handling {
		return
	}
	setProcessing(true)
	defer setProcessing(false)
	now := util.GetNowUnixMillion()
	if lastOrderTime == 0 {
		lastOrderTime = now
	}
	if now-lastOrderTime < model.AppConfig.OrderWait {
		return
	}
	timeOk, _ := carry.CheckWorthCarryTime()
	if !timeOk {
		util.SocketInfo(`get carry not on time` + carry.ToString())
		return
	}
	//if model.AppConfig.Env != `dk` {
	//	go getAccount()
	//}
	currencies := strings.Split(carry.Symbol, "_")
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
	pricePrecision := util.GetPrecision(carry.BidPrice)
	if pricePrecision > api.GetPriceDecimal(carry.BidWeb, carry.Symbol) {
		pricePrecision = api.GetPriceDecimal(carry.BidWeb, carry.Symbol)
	}
	price, _ := calcPrice(carry.BidPrice, carry.AskPrice, pricePrecision)
	util.Notice(fmt.Sprintf(`[%s] %f - %f`, carry.Symbol, leftBalance, rightBalance))
	askAmount, bidAmount := getLeftRightAmounts(leftBalance, rightBalance, carry)
	if price == carry.AskPrice || price == carry.BidPrice {
		if carry.AskAmount*100 > askAmount && carry.BidAmount*100 > bidAmount {
			util.Info(fmt.Sprintf(`[carry数量]ask:%f - %f %f bid:%f - %f %f`, askAmount,
				carry.AskAmount, carry.AskAmount/askAmount, bidAmount, carry.BidAmount, carry.BidAmount/bidAmount))
			return
		}
		if carry.AskAmount > carry.BidAmount {
			price = carry.BidPrice
		} else {
			price = carry.AskPrice
		}
	}
	model.AppMarkets.BidAsks[carry.Symbol][carry.AskWeb] = nil
	model.AppMarkets.BidAsks[carry.Symbol][carry.BidWeb] = nil
	bidAskTimes++
	if bidAskTimes%30 == 0 {
		api.RefreshAccount(model.GetMarkets()[0])
		//rebalance(leftAccount, rightAccount, carry)
		lastOrderTime = util.GetNowUnixMillion() - 5000
	} else {
		go placeRefreshOrder(carry, `buy`, `limit`, price, bidAmount)
		go placeRefreshOrder(carry, `sell`, `limit`, price, askAmount)
		time.Sleep(time.Second * 5)
	}
}

func cancelRefreshOrder(orderId string, mustCancel bool) {
	if !mustCancel {
		time.Sleep(time.Second * 5)
	}
	for i := 0; i < 100; i++ {
		market := model.GetMarkets()[0]
		settings := model.GetMarketSettings(market)
		var symbol string
		for key := range settings {
			symbol = key
			break
		}
		result, errCode, _ := api.CancelOrder(market, symbol, orderId)
		util.Notice(fmt.Sprintf(`[cancel] %s for %d times, return %t `, orderId, i, result))
		if result || !mustCancel {
			break
		} else if errCode == `429` || errCode == `4003` {
			util.Notice(`調用次數繁忙`)
		} else if i >= 3 {
			break
		}
		if i == 99 {
			model.AppConfig.Handle = 0
		}
		time.Sleep(time.Second * 1)
	}
}

func RefreshCarryServe() {
	for true {
		orderCarry := <-model.RefreshCarryChannel
		util.Notice(fmt.Sprintf(`||||||[bid-ask] [%s %s] [%s %s]`, orderCarry.DealBidOrderId,
			orderCarry.DealAskOrderId, orderCarry.DealBidStatus, orderCarry.DealAskStatus))
		if orderCarry.DealBidStatus == `` || orderCarry.DealAskStatus == `` {
			continue
		}
		handling = true
		if orderCarry.DealAskStatus == model.CarryStatusWorking && orderCarry.DealBidStatus == model.CarryStatusWorking {
			lastOrderTime = util.GetNowUnixMillion()
			go cancelRefreshOrder(orderCarry.DealAskOrderId, false)
			go cancelRefreshOrder(orderCarry.DealBidOrderId, false)
			if model.AppConfig.Env == `dk` {
				go placeExtraSell(&orderCarry)
			}
		} else if orderCarry.DealAskStatus == model.CarryStatusWorking && orderCarry.DealBidStatus == model.CarryStatusFail {
			cancelRefreshOrder(orderCarry.DealAskOrderId, true)
		} else if orderCarry.DealAskStatus == model.CarryStatusFail && orderCarry.DealBidStatus == model.CarryStatusWorking {
			cancelRefreshOrder(orderCarry.DealBidOrderId, true)
		}
		//if orderCarry.DealBidErrCode == `1002` || orderCarry.DealAskErrCode == `1002` {
		//	util.Notice(`1002系统繁忙不算时间`)
		//} else {
		//}
		handling = false
	}
}
