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

func getAccount() {
	switch model.GetMarkets()[0] {
	case model.Coinpark:
		api.GetAccountCoinpark(model.ApplicationAccounts)
	case model.Fcoin:
		api.GetAccountFcoin(model.ApplicationAccounts)
	case model.Coinbig:
		api.GetAccountCoinbig(model.ApplicationAccounts)
	}
}
func calcPrice(bidPrice, askPrice float64, precision int) (num float64, err error) {
	str := strconv.FormatFloat(bidPrice+(askPrice-bidPrice)*1/2, 'f', precision, 64)
	return strconv.ParseFloat(str, 64)
}

func getLeftRightAmounts(leftBalance, rightBalance float64, carry *model.Carry) (askAmount, bidAmount float64) {
	amountPrecision := util.GetPrecision(carry.BidAmount)
	if model.ApplicationConfig.Env == `dk` {
		if leftBalance*model.ApplicationConfig.AmountRate > rightBalance/carry.BidPrice {
			carry.Amount = rightBalance / carry.BidPrice
		} else {
			carry.Amount = leftBalance * model.ApplicationConfig.AmountRate
		}
		askAmount = carry.Amount
		bidAmount = carry.Amount
	} else {
		askAmount = leftBalance * model.ApplicationConfig.AmountRate
		bidAmount = rightBalance * model.ApplicationConfig.AmountRate / carry.BidPrice
	}
	strAskAmount := strconv.FormatFloat(askAmount, 'f', amountPrecision, 64)
	strBidAmount := strconv.FormatFloat(bidAmount, 'f', amountPrecision, 64)
	askAmount, _ = strconv.ParseFloat(strAskAmount, 64)
	bidAmount, _ = strconv.ParseFloat(strBidAmount, 64)
	return askAmount, bidAmount
}

func placeRefreshOrder(carry *model.Carry, orderSide, orderType, price, amount string) {
	if orderSide == `buy` {
		carry.DealBidOrderId, carry.DealBidErrCode, _ = api.PlaceOrder(orderSide, orderType,
			model.GetMarkets()[0], carry.Symbol, price, amount)
		if carry.DealBidOrderId != `` && carry.DealBidOrderId != "0" {
			carry.DealBidStatus = model.CarryStatusWorking
		} else {
			carry.DealBidStatus = model.CarryStatusFail
		}
		util.Notice(fmt.Sprintf(`====%s==== %s %s 价格: %s 数量: %s 返回 %s %s`,
			orderSide, orderType, carry.Symbol, price, amount, carry.DealBidOrderId, carry.DealBidErrCode))
	} else if orderSide == `sell` {
		carry.DealAskOrderId, carry.DealAskErrCode, _ = api.PlaceOrder(orderSide, orderType, carry.Symbol, orderType,
			price, amount)
		if carry.DealAskOrderId != `` && carry.DealAskOrderId != "0" {
			carry.DealAskStatus = model.CarryStatusWorking
		} else {
			carry.DealAskStatus = model.CarryStatusFail
		}
		util.Notice(fmt.Sprintf(`====%s==== %s %s 价格: %s 数量: %s 返回 %s %s`,
			orderSide, orderType, carry.Symbol, price, amount, carry.DealAskOrderId, carry.DealAskErrCode))
	}
	if carry.DealAskErrCode == `2027` || carry.DealBidErrCode == `2027` {
		go getAccount()
	}
	model.RefreshCarryChannel <- *carry
	model.CarryChannel <- *carry
}

func getDecimalLimit(symbol string) int {
	switch symbol {
	case `ft_usdt`:
		return 6
	case `ft_eth`:
		return 8
	case `ft_btc`:
		return 8
	case `eth_usdt`:
		return 2
	case `btc_usdt`:
		return 2
	}
	return 100
}
func setProcessing(value bool) {
	processing = value
}

func placeExtraSell(carry *model.Carry) {
	account := model.ApplicationAccounts.GetAccount(model.Fcoin, `ft`)
	if account == nil {
		util.Notice(`[额外卖单-nil account]`)
	} else {
		util.Notice(fmt.Sprintf(`[额外卖单]%f - %f`, account.Free, model.ApplicationConfig.FtMax))
	}
	if account != nil && account.Free > model.ApplicationConfig.FtMax {
		pricePrecision := util.GetPrecision(carry.BidPrice)
		if pricePrecision > getDecimalLimit(carry.Symbol) {
			pricePrecision = getDecimalLimit(carry.Symbol)
		}
		price := carry.BidPrice * 0.999
		strPrice := strconv.FormatFloat(price, 'f', pricePrecision, 64)
		amount := carry.Amount * model.ApplicationConfig.SellRate
		strAmount := strconv.FormatFloat(amount, 'f', 2, 64)
		orderId, errCode, msg := api.PlaceOrder(model.OrderSideSell, model.OrderTypeLimit,
			model.GetMarkets()[0], carry.Symbol, strPrice, strAmount)
		util.Notice(fmt.Sprintf(`[额外卖单]%s 价格: %s 数量: %s 返回 %s %s %s`,
			carry.Symbol, strPrice, strAmount, orderId, errCode, msg))
	}
}

var ProcessRefresh = func(symbol, market string) {
	carry, err := model.ApplicationMarkets.NewCarry(symbol)
	if err != nil {
		util.Notice(`can not create carry for ` + symbol)
		return
	}
	if model.ApplicationConfig.Handle == 0 || processing || handling {
		return
	}
	setProcessing(true)
	defer setProcessing(false)
	now := util.GetNowUnixMillion()
	if lastOrderTime == 0 {
		lastOrderTime = now
	}
	if now-lastOrderTime < model.ApplicationConfig.OrderWait {
		return
	}
	timeOk, _ := carry.CheckWorthCarryTime(model.ApplicationMarkets, model.ApplicationConfig)
	if !timeOk {
		util.SocketInfo(`get carry not on time` + carry.ToString())
		return
	}
	//if model.ApplicationConfig.Env != `dk` {
	//	go getAccount()
	//}
	currencies := strings.Split(carry.Symbol, "_")
	leftAccount := model.ApplicationAccounts.GetAccount(carry.AskWeb, currencies[0])
	if leftAccount == nil {
		util.Notice(`nil account ` + carry.AskWeb + currencies[0])
		//go getAccount()
		return
	}
	leftBalance := leftAccount.Free
	rightAccount := model.ApplicationAccounts.GetAccount(carry.BidWeb, currencies[1])
	if rightAccount == nil {
		util.Notice(`nil account ` + carry.BidWeb + currencies[1])
		//go getAccount()
		return
	}
	rightBalance := rightAccount.Free
	pricePrecision := util.GetPrecision(carry.BidPrice)
	if pricePrecision > getDecimalLimit(carry.Symbol) {
		pricePrecision = getDecimalLimit(carry.Symbol)
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
	strAskAmount := strconv.FormatFloat(askAmount, 'f', -1, 64)
	strBidAmount := strconv.FormatFloat(bidAmount, 'f', -1, 64)
	//carry.AskPrice = price
	//carry.BidPrice = price
	strPrice := strconv.FormatFloat(price, 'f', -1, 64)
	model.ApplicationMarkets.BidAsks[carry.Symbol][carry.AskWeb] = nil
	model.ApplicationMarkets.BidAsks[carry.Symbol][carry.BidWeb] = nil
	bidAskTimes++
	if bidAskTimes%30 == 0 {
		getAccount()
		//rebalance(leftAccount, rightAccount, carry)
		lastOrderTime = util.GetNowUnixMillion() - 5000
	} else {
		go placeRefreshOrder(carry, `buy`, `limit`, strPrice, strBidAmount)
		go placeRefreshOrder(carry, `sell`, `limit`, strPrice, strAskAmount)
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
			model.ApplicationConfig.Handle = 0
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
			if model.ApplicationConfig.Env == `dk` {
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
