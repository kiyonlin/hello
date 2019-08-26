package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"strconv"
	"strings"
	"sync"
)

var rank = &Rank{}
var minPoint = 0.0001

type Rank struct {
	lock sync.Mutex
	//ranking bool
	checkTime map[string]int64          // symbol - time
	orders    map[string][]*model.Order // symbol - orders
	score     map[string]*model.Score   // symbol - score
}

func (rank *Rank) getCheckTime(symbol string) (checkTime int64) {
	if rank.checkTime == nil {
		return 0
	}
	return rank.checkTime[symbol]
}

func (rank *Rank) setCheckTime(symbol string) {
	rank.lock.Lock()
	defer rank.lock.Unlock()
	if rank.checkTime == nil {
		rank.checkTime = make(map[string]int64)
	}
	rank.checkTime[symbol] = util.GetNowUnixMillion()
}

func (rank *Rank) getScore(orderSide, symbol string) (score *model.Score) {
	rank.lock.Lock()
	defer rank.lock.Unlock()
	if rank.score == nil {
		return nil
	}
	return rank.score[symbol]
}

func (rank *Rank) getOrders(symbol string) (order []*model.Order) {
	rank.lock.Lock()
	defer rank.lock.Unlock()
	if rank.orders == nil {
		return nil
	}
	return rank.orders[symbol]
}

func (rank *Rank) setOrders(symbol string, orders []*model.Order) {
	rank.lock.Lock()
	defer rank.lock.Unlock()
	if rank.orders == nil {
		rank.orders = make(map[string][]*model.Order)
	}
	rank.orders[symbol] = orders
}

var ProcessRank = func(market, symbol string) {
	result, tick := model.AppMarkets.GetBidAsk(symbol, market)
	if !result || tick == nil || tick.Asks == nil || tick.Bids == nil || tick.Asks.Len() < 11 ||
		tick.Bids.Len() < 11 {
		util.Notice(fmt.Sprintf(`[tick not good]%s %s`, market, symbol))
		return
	}
	if model.AppConfig.Handle != `1` || model.AppPause {
		return
	}
	delay := util.GetNowUnixMillion() - int64(tick.Ts)
	if delay > 500 {
		util.Notice(fmt.Sprintf(`%s %s [delay too long] %d`, market, symbol, delay))
		return
	}
	priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	checkDistance := priceDistance / 10
	completeTick(market, symbol, tick, priceDistance, checkDistance)
	settings := model.GetFunctionMarketSettings(model.FunctionRank, market)
	setting := model.GetSetting(model.FunctionRank, market, symbol)
	if setting == nil || settings == nil {
		return
	}
	didSmth := false
	orders := rank.getOrders(symbol)
	newOrders := make([]*model.Order, 0)
	for _, order := range orders {
		orderScore := calcOrderScore(order, setting, tick)
		if orderScore.Point > minPoint {
			newOrders = append(newOrders, order)
		} else if (order.OrderSide == model.OrderSideBuy && order.Price < tick.Bids[0].Price+checkDistance) ||
			(order.OrderSide == model.OrderSideSell && order.Price > tick.Asks[0].Price-checkDistance) {
			api.MustCancel(``, ``, market, symbol, order.OrderId, false)
			didSmth = true
		}
	}
	if !didSmth {
		score := calcHighestScore(setting, tick)
		if (score.OrderSide == model.OrderSideBuy && score.Point > setting.OpenShortMargin) ||
			(score.OrderSide == model.OrderSideSell && score.Point > setting.CloseShortMargin) {
			leftFree, rightFree, _, _, _ := getBalance(key, secret, market, symbol, setting.AccountType)
			if model.AppConfig.FcoinKey == `` {
				model.AppDB.Save(&score)
			} else {
				if score.OrderSide == model.OrderSideSell {
					if score.Amount < leftFree {
						api.PlaceOrder(``, ``, score.OrderSide, model.OrderTypeLimit, market,
							symbol, ``, setting.AccountType, score.Price, score.Amount)
					}
					priceOppo := tick.Asks[0].Price - priceDistance
					if score.Point > setting.OpenShortMargin && score.Point > setting.CloseShortMargin &&
						score.Amount < rightFree/priceOppo {
						api.PlaceOrder(``, ``, model.OrderSideBuy, model.OrderTypeLimit, market,
							symbol, ``, setting.AccountType, priceOppo, score.Amount)
					}
				} else if score.OrderSide == model.OrderSideBuy {
					if score.Amount < rightFree/score.Price {
						api.PlaceOrder(``, ``, score.OrderSide, model.OrderTypeLimit, market,
							symbol, ``, setting.AccountType, score.Price, score.Amount)
					}
					priceOppo := tick.Bids[0].Price + priceDistance
					if score.Point > setting.OpenShortMargin && score.Point > setting.CloseShortMargin &&
						score.Amount < leftFree {
						api.PlaceOrder(``, ``, model.OrderSideSell, model.OrderTypeLimit, market,
							symbol, ``, setting.AccountType, priceOppo, score.Amount)
					}
				}
			}
		}
	}
	if util.GetNowUnixMillion()-rank.getCheckTime(symbol) > 300000 {
		newOrders = api.QueryOrders(``, ``, market, symbol, model.CarryStatusWorking, setting.AccountType,
			0, 0)
		util.Info(fmt.Sprintf(`get working orders from api %s %d`, symbol, len(newOrders)))
		rank.setCheckTime(symbol)
	}
	rank.setOrders(symbol, newOrders)
}

func completeTick(market, symbol string, tick *model.BidAsk, priceDistance, checkDistance float64) {
	newBids := make([]model.Tick, 11)
	newAsks := make([]model.Tick, 11)
	basePrice := tick.Bids[0].Price
	for i := 0; i < 11; i++ {
		askPrice, _ := util.FormatNum(basePrice+priceDistance*float64(i+1), api.GetPriceDecimal(market, symbol))
		bidPrice, _ := util.FormatNum(basePrice-priceDistance*float64(i), api.GetPriceDecimal(market, symbol))
		newAsks[i] = model.Tick{Symbol: symbol, Price: askPrice, Amount: 0}
		newBids[i] = model.Tick{Symbol: symbol, Price: bidPrice, Amount: 0}
	}
	posBid := 0
	posAsk := 0
	for i := 0; i < 11; i++ {
		for ; posBid < 11; posBid++ {
			if math.Abs(tick.Bids[posBid].Price-newBids[i].Price) < checkDistance {
				newBids[i].Amount = tick.Bids[posBid].Amount
				break
			}
		}
		for ; posAsk < 11; posAsk++ {
			if math.Abs(tick.Asks[posAsk].Price-newAsks[i].Price) < checkDistance {
				newAsks[i].Amount = tick.Asks[posAsk].Amount
				break
			}
		}
	}
	tick.Asks = newAsks
	tick.Bids = newBids
}

func calcHighestScore(setting *model.Setting, tick *model.BidAsk) (score *model.Score) {
	coins := strings.Split(setting.Symbol, `_`)
	perUsdt, _ := api.GetPrice(``, ``, coins[1]+`_usdt`)
	rankFt, err := strconv.ParseFloat(setting.FunctionParameter, 10)
	if err != nil {
		util.Notice(`rank function parameter err ` + setting.FunctionParameter)
		return nil
	}
	minAmount := api.GetMinAmount(setting.Market, setting.Symbol)
	for key, value := range tick.Bids {
		if value.Amount < minAmount {
			tick.Bids[key].Amount += minAmount
		} else {
			tick.Bids[key].Amount *= 2
		}
	}
	for key, value := range tick.Asks {
		if value.Amount < minAmount {
			tick.Asks[key].Amount += minAmount
		} else {
			tick.Asks[key].Amount *= 2
		}
	}
	rankFt = rankFt / 5760
	score = &model.Score{Symbol: setting.Symbol, OrderSide: model.OrderSideBuy, Amount: tick.Bids[0].Amount,
		Price: tick.Bids[0].Price, Point: rankFt / (tick.Bids[0].Price * tick.Bids[0].Amount * perUsdt), Position: 0}
	if tick.Bids[0].Amount > tick.Asks[0].Amount {
		score = &model.Score{Symbol: setting.Symbol, OrderSide: model.OrderSideSell, Amount: tick.Asks[0].Amount,
			Price: tick.Asks[0].Price, Point: rankFt / (tick.Asks[0].Price * tick.Asks[0].Amount * perUsdt), Position: 0}
	}
	score10 := &model.Score{Symbol: setting.Symbol, OrderSide: model.OrderSideBuy,
		Amount: tick.Bids[1].Amount, Price: tick.Bids[1].Price}
	for i := 1; i < 11; i++ {
		if score10.Amount > tick.Bids[i].Amount {
			score10.OrderSide = model.OrderSideBuy
			score10.Amount = tick.Bids[i].Amount
			score10.Price = tick.Bids[i].Price
			score10.Position = i
		}
		if score10.Amount > tick.Asks[i].Amount {
			score10.OrderSide = model.OrderSideSell
			score10.Amount = tick.Asks[i].Amount
			score10.Price = tick.Asks[i].Price
			score10.Position = i
		}
	}
	score10.Point = rankFt / 10 / (score10.Price * score10.Amount * perUsdt)
	if score10.Point > score.Point {
		return score10
	}
	return score
}

func calcOrderScore(order *model.Order, setting *model.Setting, tick *model.BidAsk) (score *model.Score) {
	if order.Price < tick.Bids[10].Price || order.Price > tick.Asks[10].Price ||
		(order.OrderSide == model.OrderSideBuy && order.Price > tick.Bids[0].Price) ||
		(order.OrderSide == model.OrderSideSell && order.Price < tick.Asks[0].Price) {
		return &model.Score{Point: 0}
	}
	coins := strings.Split(setting.Symbol, `_`)
	perUsdt, _ := api.GetPrice(``, ``, coins[1]+`_usdt`)
	rankFt, err := strconv.ParseFloat(setting.FunctionParameter, 10)
	if err != nil {
		util.Notice(`rank function parameter err ` + setting.FunctionParameter)
		return &model.Score{Point: 0}
	}
	rankFt = rankFt / 5760
	score = &model.Score{Symbol: setting.Symbol, OrderSide: order.OrderSide, Price: order.Price, Point: 0}
	if order.OrderSide == model.OrderSideBuy && order.Price == tick.Bids[0].Price {
		score.Amount = tick.Bids[0].Amount
		score.Point = rankFt / (tick.Bids[0].Price * tick.Bids[0].Amount * perUsdt)
	} else if order.OrderSide == model.OrderSideSell && order.Price == tick.Asks[0].Price {
		score.Amount = tick.Asks[0].Amount
		score.Point = rankFt / (tick.Asks[0].Price * tick.Asks[0].Amount * perUsdt)
	} else {
		for i := 1; i < 11; i++ {
			if order.OrderSide == model.OrderSideBuy && order.Price == tick.Bids[i].Price {
				score.Amount = tick.Bids[i].Amount
				score.Point = rankFt / 10 / (tick.Bids[i].Price * tick.Bids[i].Amount * perUsdt)
				break
			}
			if order.OrderSide == model.OrderSideSell && order.Price == tick.Asks[i].Price {
				score.Amount = tick.Asks[i].Amount
				score.Point = rankFt / 10 / (tick.Asks[i].Price * tick.Asks[i].Amount * perUsdt)
				break
			}
		}
	}
	return score
}

func recalcRankLine(market string) (settings map[string]*model.Setting) {
	settings = model.GetFunctionMarketSettings(model.FunctionRank, market)
	weight := make(map[string]float64)
	for symbol := range settings {
		coins := strings.Split(symbol, `_`)
		weight[coins[0]] = weight[coins[0]] + 1/(1+weight[coins[0]])
		weight[coins[1]] = weight[coins[1]] + 1/(1+weight[coins[1]])
	}
	amount := make(map[string]float64)
	for key, value := range weight {
		account := model.AppAccounts.GetAccount(market, key)
		if account != nil && value > 0 {
			price, _ := api.GetPrice(``, ``, key+`_usdt`)
			amount[key] = account.Free * price
		}
	}
	for symbol, setting := range settings {
		coins := strings.Split(symbol, `_`)
		rate := (amount[coins[0]] / weight[coins[0]]) / (amount[coins[1]] / weight[coins[1]])
		all := setting.OpenShortMargin + setting.CloseShortMargin
		setting.OpenShortMargin = all * rate / (1 + rate)
		setting.CloseShortMargin = all / (1 + rate)
		util.Info(fmt.Sprintf(`%s open: %f close: %f %f %s %f %f %s %f %f`, symbol, setting.OpenShortMargin,
			setting.CloseShortMargin, rate, coins[0], amount[coins[0]], weight[coins[0]],
			coins[1], amount[coins[1]], weight[coins[1]]))
	}
	return settings
}
