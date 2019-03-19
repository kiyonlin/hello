package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"strings"
)

// fcoin:// 下單返回1016 資金不足// 下单返回1002 系统繁忙// 返回426 調用次數太頻繁
// coinpark://4003 调用次数繁忙 //2085 最小下单数量限制 //2027 可用余额不足
var bidAskTimes int64
var processing = false
var refreshing = false
var snycRefresh = make(chan interface{}, 10)
var refreshLastBid, refreshLastAsk *model.Order

func setRefreshing(value bool) {
	refreshing = value
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

var ProcessRefresh func(market string, symbol string) = func(market, symbol string) {
	if model.AppConfig.Handle != `1` || model.AppConfig.HandleRefresh != `1` || processing || refreshing {
		return
	}
	setRefreshing(true)
	defer setRefreshing(false)
	currencies := strings.Split(symbol, "_")
	leftAccount := model.AppAccounts.GetAccount(market, currencies[0])
	if leftAccount == nil {
		util.Notice(`nil account ` + market + currencies[0])
		//go getAccount()
		return
	}
	leftBalance := leftAccount.Free
	rightAccount := model.AppAccounts.GetAccount(market, currencies[1])
	if rightAccount == nil {
		util.Notice(`nil account ` + market + currencies[1])
		//go getAccount()
		return
	}
	rightBalance := rightAccount.Free
	if model.AppMarkets.BidAsks[symbol] == nil || model.AppMarkets.BidAsks[symbol][market] == nil ||
		len(model.AppMarkets.BidAsks[symbol][market].Bids) == 0 || len(model.AppMarkets.BidAsks[symbol][market].Asks) == 0 {
		util.Notice(`nil bid-ask price for ` + symbol)
		return
	}
	bidPrice := model.AppMarkets.BidAsks[symbol][market].Bids[0].Price
	askPrice := model.AppMarkets.BidAsks[symbol][market].Asks[0].Price
	bidAmount := model.AppMarkets.BidAsks[symbol][market].Bids[0].Amount
	askAmount := model.AppMarkets.BidAsks[symbol][market].Asks[0].Amount
	price := (bidPrice + askPrice) / 2
	util.Notice(fmt.Sprintf(`[%s] %f - %f`, symbol, leftBalance, rightBalance))
	amount := math.Min(leftBalance, rightBalance/price) * model.AppConfig.AmountRate
	priceDistance := 0.5 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	if (price-bidPrice) < priceDistance || (askPrice-price) < priceDistance {
		if askAmount > bidAmount {
			price = bidPrice
			if bidAmount*10 > amount {
				util.Notice(fmt.Sprintf(`[refresh crash]bid:%f - %f`, bidAmount, amount))
				return
			}
		} else {
			price = askPrice
			if askAmount*10 > amount {
				util.Notice(fmt.Sprintf(`[refresh crash]ask:%f - %f`, askAmount, amount))
				return
			}
		}
	}
	bidAskTimes++
	if bidAskTimes%7 == 0 {
		api.RefreshAccount(market)
		//rebalance(leftAccount, rightAccount, carry)
	}
	if refreshLastBid == nil || refreshLastAsk == nil ||
		bidPrice < refreshLastBid.Price || askPrice > refreshLastAsk.Price {
		refreshLastBid = nil
		refreshLastAsk = nil
		go placeRefreshOrder(model.OrderSideSell, market, symbol, price, amount)
		go placeRefreshOrder(model.OrderSideBuy, market, symbol, price, amount)
		for true {
			<-snycRefresh
			if refreshLastBid != nil && refreshLastAsk != nil {
				if refreshLastBid.Status == model.CarryStatusWorking && refreshLastAsk.Status == model.CarryStatusFail {
					api.MustCancel(refreshLastBid.Market, refreshLastBid.Symbol, refreshLastBid.OrderId, true)
					refreshLastBid = nil
					refreshLastAsk = nil
				}
				if refreshLastAsk.Status == model.CarryStatusWorking && refreshLastBid.Status == model.CarryStatusFail {
					api.MustCancel(refreshLastAsk.Market, refreshLastAsk.Symbol, refreshLastAsk.OrderId, true)
					refreshLastBid = nil
					refreshLastAsk = nil
				}
				if refreshLastAsk.Status == model.CarryStatusFail && refreshLastBid.Status == model.CarryStatusFail {
					refreshLastBid = nil
					refreshLastAsk = nil
				}
				break
			}
		}
	} else {
		if bidPrice > refreshLastBid.Price || askPrice < refreshLastAsk.Price {
			api.CancelOrder(market, symbol, refreshLastAsk.OrderId)
			api.CancelOrder(market, symbol, refreshLastBid.OrderId)
			refreshLastBid = nil
			refreshLastAsk = nil
		}
	}
}

func placeRefreshOrder(orderSide, market, symbol string, price, amount float64) {
	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, price, amount)
	order.Function = model.FunctionRefresh
	if orderSide == model.OrderSideBuy {
		refreshLastBid = order
	}
	if orderSide == model.OrderSideSell {
		refreshLastAsk = order
	}
	model.AppDB.Save(order)
	snycRefresh <- struct{}{}
}
