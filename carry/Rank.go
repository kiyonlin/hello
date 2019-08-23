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
			go model.AppDB.Save(&score)
			leftFree, rightFree, _, _, _ := getBalance(key, secret, market, symbol, setting.AccountType)
			if model.AppConfig.FcoinKey != `` &&
				((score.OrderSide == model.OrderSideSell && score.Amount < leftFree) ||
					(score.OrderSide == model.OrderSideBuy && score.Amount < rightFree/tick.Asks[0].Price)) {
				score.Amount = math.Max(api.GetMinAmount(market, symbol), score.Amount)
				order := api.PlaceOrder(``, ``, score.OrderSide, model.OrderTypeLimit, market, symbol,
					``, setting.AccountType, score.Price, score.Amount)
				if order.OrderId != `` {
					order.Function = model.FunctionRank
					order.Status = model.CarryStatusSuccess
					go model.AppDB.Save(&order)
					newOrders = append(newOrders, order)
				} else if order.ErrCode == `1016` {
					coins := strings.Split(symbol, `_`)
					if score.OrderSide == model.OrderSideBuy {
						util.Info(fmt.Sprintf(`%s %f %s not enough<%f`, coins[1], score.Point, symbol, score.Amount))
					} else {
						util.Info(fmt.Sprintf(`%s %f %s not enough<%f`, coins[0], score.Point, symbol, score.Amount))
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
	maxValue := 0.0
	minValue := 0.0
	maxCoin := ``
	minCoin := ``
	settings = model.GetFunctionMarketSettings(model.FunctionRank, market)
	coinCount := make(map[string]float64)
	for symbol := range settings {
		coins := strings.Split(symbol, `_`)
		coinCount[coins[0]] = coinCount[coins[0]] + 1/(1+coinCount[coins[0]])
		coinCount[coins[1]] = coinCount[coins[1]] + 1/(1+coinCount[coins[1]])
	}
	for key, value := range coinCount {
		account := model.AppAccounts.GetAccount(market, key)
		if account != nil && value > 0 {
			price, _ := api.GetPrice(``, ``, key+`_usdt`)
			coinCount[key] = account.Free * price / value
		}
	}
	for key, value := range coinCount {
		if value > maxValue {
			maxValue = value
			maxCoin = key
		}
		if value < minValue || minValue == 0 {
			minValue = value
			minCoin = key
		}
	}
	for symbol, setting := range settings {
		coins := strings.Split(symbol, `_`)
		if setting.OpenShortMargin > setting.Chance+minPoint && setting.CloseShortMargin > setting.Chance+minPoint {
			if coins[0] == minCoin {
				setting.OpenShortMargin -= setting.Chance
				setting.CloseShortMargin += setting.Chance
			} else if coins[0] == maxCoin {
				setting.OpenShortMargin += setting.Chance
				setting.CloseShortMargin -= setting.Chance
			}
			if coins[1] == minCoin {
				setting.OpenShortMargin += setting.Chance
				setting.CloseShortMargin -= setting.Chance
			} else if coins[1] == maxCoin {
				setting.OpenShortMargin -= setting.Chance
				setting.CloseShortMargin += setting.Chance
			}
		}
	}
	return settings
}
