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
	"time"
)

var rank = &Rank{}

type Rank struct {
	lock    sync.Mutex
	ranking bool
	orders  map[string][]*model.Order // symbol - orders
	score   map[string]*model.Score   // symbol - score
}

func (rank *Rank) setRanking(value bool) {
	rank.ranking = value
}

func (rank *Rank) setScore(symbol string, score *model.Score) {
	rank.lock.Lock()
	defer rank.lock.Unlock()
	if rank.score == nil {
		rank.score = make(map[string]*model.Score)
	}
	rank.score[symbol] = score
}

func (rank *Rank) getHighest(settings map[string]*model.Setting) (highest *model.Score) {
	rank.lock.Lock()
	defer rank.lock.Unlock()
	if rank.score == nil {
		return nil
	}
	for symbol := range settings {
		if rank.score[symbol] == nil {
			return nil
		} else {
			if highest == nil || highest.Point < rank.score[symbol].Point {
				highest = rank.score[symbol]
			}
		}
	}
	return highest
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
	if rank.ranking || model.AppConfig.Handle != `1` || model.AppPause {
		return
	}
	rank.setRanking(true)
	defer rank.setRanking(false)
	delay := util.GetNowUnixMillion() - int64(tick.Ts)
	if delay > 500 {
		util.Notice(fmt.Sprintf(`%s %s [delay too long] %d`, market, symbol, delay))
		return
	}
	settings := model.GetFunctionMarketSettings(model.FunctionRank, market)
	setting := model.GetSetting(model.FunctionRank, market, symbol)
	if setting == nil || settings == nil {
		return
	}
	leftFree, rightFree, _, _, err := getBalance(key, secret, market, symbol, setting.AccountType)
	if err != nil {
		return
	}
	didSmth := false
	score := calcHighestScore(setting, tick)
	rank.setScore(symbol, score)
	highest := rank.getHighest(settings)
	if highest == nil {
		return
	} else {
		go model.AppDB.Save(&highest)
	}
	orders := rank.getOrders(symbol)
	newOrders := make([]*model.Order, 0)
	for _, order := range orders {
		orderScore := calcOrderScore(order, setting, tick)
		if orderScore.Point > highest.Point/2 {
			newOrders = append(newOrders, order)
		} else if (order.OrderSide == model.OrderSideBuy && order.Price <= tick.Bids[0].Price) ||
			(order.OrderSide == model.OrderSideSell && order.Price >= tick.Asks[0].Price) {
			api.MustCancel(``, ``, market, symbol, order.OrderId, false)
			didSmth = true
		}
	}
	if didSmth {
		time.Sleep(time.Second)
		api.RefreshAccount(``, ``, market)
	} else {
		if symbol == highest.Symbol {
			if highest.OrderSide == model.OrderSideBuy {
				if rightFree/highest.Price < highest.Amount/10 {
					util.Notice(fmt.Sprintf(`----err1 amount not enough %s %s %f 小于十分之一 %f`,
						symbol, highest.OrderSide, rightFree/highest.Price, highest.Amount))
				} else {
					order := api.PlaceOrder(``, ``, highest.OrderSide, model.OrderTypeLimit, market, symbol,
						``, setting.AccountType, highest.Price, math.Min(highest.Amount, rightFree/highest.Price))
					if order.OrderId != `` {
						newOrders = append(newOrders, order)
					}
				}
			} else if highest.OrderSide == model.OrderSideSell {
				if leftFree < highest.Amount/10 {
					util.Notice(fmt.Sprintf(`----err1 amount not enough %s %s %f 小于十分之一 %f`,
						symbol, highest.OrderSide, rightFree/highest.Price, highest.Amount))
				} else {
					order := api.PlaceOrder(``, ``, highest.OrderSide, model.OrderTypeLimit, market, symbol,
						``, setting.AccountType, highest.Price, math.Min(highest.Amount, leftFree))
					if order.OrderId != `` {
						newOrders = append(newOrders, order)
					}
				}
			}
		}
	}
	rank.setOrders(symbol, newOrders)
}

func calcHighestScore(setting *model.Setting, tick *model.BidAsk) (score *model.Score) {
	coins := strings.Split(setting.Symbol, `_`)
	perUsdt, _ := api.GetPrice(``, ``, coins[1]+`_usdt`)
	rankFt, err := strconv.ParseFloat(setting.FunctionParameter, 10)
	if err != nil {
		util.Notice(`rank function parameter err ` + setting.FunctionParameter)
		return nil
	}
	rankFt = rankFt / 5760
	score = &model.Score{Symbol: setting.Symbol, OrderSide: model.OrderSideBuy, Amount: tick.Bids[0].Amount,
		Price: tick.Bids[0].Price, Point: rankFt / (tick.Bids[0].Price * tick.Bids[0].Amount * perUsdt)}
	if tick.Bids[0].Amount > tick.Asks[0].Amount {
		score = &model.Score{Symbol: setting.Symbol, OrderSide: model.OrderSideSell, Amount: tick.Asks[0].Amount,
			Price: tick.Asks[0].Price, Point: rankFt / (tick.Asks[0].Price * tick.Asks[0].Amount * perUsdt)}
	}
	score10 := &model.Score{Symbol: setting.Symbol, OrderSide: model.OrderSideBuy,
		Amount: tick.Bids[1].Amount, Price: tick.Bids[1].Price}
	for i := 1; i < 11; i++ {
		if score10.Amount > tick.Bids[i].Amount {
			score10.OrderSide = model.OrderSideBuy
			score10.Amount = tick.Bids[i].Amount
			score10.Price = tick.Bids[i].Price
		}
		if score10.Amount > tick.Asks[i].Amount {
			score10.OrderSide = model.OrderSideSell
			score10.Amount = tick.Asks[i].Amount
			score10.Price = tick.Asks[i].Price
		}
	}
	score10.Point = rankFt / 10 / (score10.Price * score10.Amount * perUsdt)
	if score10.Point > score.Point {
		return score10
	}
	return score
}

func calcOrderScore(order *model.Order, setting *model.Setting, tick *model.BidAsk) (score *model.Score) {
	if order.Price < tick.Bids[10].Price || order.Price > tick.Asks[10].Price {
		return &model.Score{Point: 0}
	}
	coins := strings.Split(setting.Symbol, `_`)
	perUsdt, _ := api.GetPrice(``, ``, coins[1]+`_usdt`)
	rankFt, err := strconv.ParseFloat(setting.FunctionParameter, 10)
	if err != nil {
		util.Notice(`rank function parameter err ` + setting.FunctionParameter)
		return nil
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