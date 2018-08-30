package model

import (
	"fmt"
	"github.com/pkg/errors"
	"hello/util"
	"math"
	"strconv"
	"strings"
	"time"
)

type Carry struct {
	Symbol         string
	BidWeb         string
	AskWeb         string
	BidAmount      float64
	AskAmount      float64
	Amount         float64
	DealBidAmount  float64
	DealAskAmount  float64
	BidPrice       float64
	AskPrice       float64
	DealBidErrCode string
	DealBidOrderId string
	DealAskErrCode string
	DealAskOrderId string
	DealBidStatus  string
	DealAskStatus  string
	Margin         float64
	// time_idx的设计有一定的冲突风险，但为了在发起挂单前减少一次db操作而不使用carry的id
	BidTime   int64
	AskTime   int64
	ID        uint `gorm:"primary_key"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SideType  string
}

func getDynamicMargin(carry *Carry, configMargin float64) (dynamicMargin float64) {
	if AppAccounts == nil {
		return configMargin
	}
	if !strings.Contains(carry.Symbol, "_") {
		util.Info(fmt.Sprintf("invalid carry %s %s %f %d", carry.Symbol, carry.BidWeb, carry.Amount, carry.BidTime))
		return 1
	}
	currencies := strings.Split(carry.Symbol, "_")
	leftTotalPercentage := AppAccounts.CurrencyPercentage[currencies[0]]
	rightTotalPercentage := AppAccounts.CurrencyPercentage[currencies[1]]
	if leftTotalPercentage == 0 || rightTotalPercentage == 0 {
		return configMargin
	}
	var leftAskPercentage, rightAskPercentage, leftBidPercentage, rightBidPercentage float64
	leftAskAccount := AppAccounts.GetAccount(carry.AskWeb, currencies[0])
	rightAskAccount := AppAccounts.GetAccount(carry.AskWeb, currencies[1])
	leftBidAccount := AppAccounts.GetAccount(carry.BidWeb, currencies[0])
	rightBidAccount := AppAccounts.GetAccount(carry.BidWeb, currencies[1])
	if leftAskAccount != nil {
		// 计算假如本次carry完成后达到的占比
		leftAskPercentage = (leftAskAccount.Free + leftAskAccount.Frozen - carry.Amount) *
			leftAskAccount.PriceInUsdt / AppAccounts.MarketTotal[carry.AskWeb]
	}
	if rightAskAccount != nil {
		rightAskPercentage = (rightAskAccount.Free + rightAskAccount.Free + carry.Amount) *
			rightAskAccount.PriceInUsdt / AppAccounts.MarketTotal[carry.AskWeb]
	}
	if leftBidAccount != nil {
		leftBidPercentage = (leftBidAccount.Free + leftBidAccount.Frozen + carry.Amount) *
			leftBidAccount.PriceInUsdt / AppAccounts.MarketTotal[carry.BidWeb]
	}
	if rightBidAccount != nil {
		rightBidPercentage = (rightBidAccount.Free + rightBidAccount.Frozen - carry.Amount) *
			rightBidAccount.PriceInUsdt / AppAccounts.MarketTotal[carry.BidWeb]
	}
	if leftAskPercentage >= leftTotalPercentage && rightAskPercentage <= rightTotalPercentage && leftBidPercentage <=
		leftTotalPercentage && rightBidPercentage >= rightTotalPercentage {
		discount := (rightTotalPercentage - rightAskPercentage) / rightTotalPercentage
		if discount < (leftTotalPercentage-leftBidPercentage)/leftTotalPercentage {
			discount = (leftTotalPercentage - leftBidPercentage) / leftTotalPercentage
		}
		askCost, _ := AppConfig.MarketCost[carry.AskWeb]
		bidCost, _ := AppConfig.MarketCost[carry.BidWeb]
		baseCost := (askCost + bidCost) / 2
		dynamicMargin := baseCost + (configMargin-baseCost)*(1-discount)
		util.SocketInfo(fmt.Sprintf("%s -> %s %s discount:%f实际门槛 %f", carry.AskWeb, carry.BidWeb,
			carry.Symbol, discount, dynamicMargin))
		return dynamicMargin
	}
	return configMargin
}

func (carry *Carry) CheckWorthSaveMargin() bool {
	askCost, _ := AppConfig.MarketCost[carry.AskWeb]
	bidCost, _ := AppConfig.MarketCost[carry.BidWeb]
	if (askCost+bidCost)/2 < (carry.AskPrice-carry.BidPrice)/carry.AskPrice {
		return true
	}
	return false
}

func (carry *Carry) CheckWorthCarryMargin(markets *Markets, config *Config) (bool, error) {
	dynamicMargin := getDynamicMargin(carry, GetMargin(carry.Symbol))
	carry.Margin = dynamicMargin
	margin := carry.AskPrice - carry.BidPrice
	if margin > carry.AskPrice*dynamicMargin && carry.Amount > 0 {
		util.Info(fmt.Sprintf("利润门槛:%.4f 值得搬砖%s", dynamicMargin, carry.ToString()))
		return true, nil
	}
	return false, errors.New("利润不足")
}

func (carry *Carry) CheckWorthCarryTime() (bool, error) {
	now := util.GetNowUnixMillion()
	bidTimeDelay := math.Abs(float64(now - carry.BidTime))
	askTimeDelay := math.Abs(float64(now - carry.AskTime))
	timeDiff := math.Abs(float64(carry.BidTime - carry.AskTime))
	if timeDiff > AppConfig.Delay || bidTimeDelay > AppConfig.Delay || askTimeDelay > AppConfig.Delay {
		message := strconv.Itoa(int(now)) + "时间问题，卖方" + carry.AskWeb + strconv.Itoa(int(carry.AskTime)) + "隔" +
			strconv.Itoa(int(askTimeDelay))
		message = message + "买方" + carry.BidWeb + strconv.Itoa(int(carry.BidTime)) + "隔" + strconv.Itoa(int(bidTimeDelay))
		return false, errors.New(message)
	}
	return true, nil
}

func (carry *Carry) ToString() string {
	return fmt.Sprintf("%s [%s->%s] 间隔%d - %d 数量%f ask %f bid %f 利润%f - %f 价格%f - %f",
		carry.Symbol, carry.AskWeb, carry.BidWeb, time.Now().Unix()*1000-carry.AskTime,
		time.Now().Unix()*1000-carry.BidTime, carry.Amount, carry.AskAmount, carry.BidAmount,
		(carry.AskPrice-carry.BidPrice)/carry.AskPrice, carry.Margin, carry.BidPrice, carry.AskPrice)
}
