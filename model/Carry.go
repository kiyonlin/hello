package model

import (
	"github.com/pkg/errors"
	"math"
	"strconv"
	"hello/util"
	"time"
	"strings"
	"fmt"
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
	BidTime   int64 `gorm:"unique_index:time_idx;"`
	AskTime   int64 `gorm:"unique_index:time_idx;"`
	ID        uint  `gorm:"primary_key"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func getDynamicMargin(carry *Carry, configMargin float64) (dynamicMargin float64) {
	if ApplicationAccounts == nil {
		return configMargin
	}
	if !strings.Contains(carry.Symbol, "_") {
		util.Notice(fmt.Sprintf("invalid carry %s %s %f %d", carry.Symbol, carry.BidWeb, carry.Amount, carry.BidTime))
		return 1
	}
	currencies := strings.Split(carry.Symbol, "_")
	leftTotalPercentage := ApplicationAccounts.CurrencyPercentage[currencies[0]]
	rightTotalPercentage := ApplicationAccounts.CurrencyPercentage[currencies[1]]
	if leftTotalPercentage == 0 || rightTotalPercentage == 0 {
		return configMargin
	}
	var leftAskPercentage, rightAskPercentage, leftBidPercentage, rightBidPercentage float64
	leftAskAccount := ApplicationAccounts.GetAccount(carry.AskWeb, currencies[0])
	rightAskAccount := ApplicationAccounts.GetAccount(carry.AskWeb, currencies[1])
	leftBidAccount := ApplicationAccounts.GetAccount(carry.BidWeb, currencies[0])
	rightBidAccount := ApplicationAccounts.GetAccount(carry.BidWeb, currencies[1])
	if leftAskAccount != nil {
		// 计算假如本次carry完成后达到的占比
		leftAskPercentage = (leftAskAccount.Free + leftAskAccount.Frozen - carry.Amount) *
			leftAskAccount.PriceInUsdt / ApplicationAccounts.MarketTotal[carry.AskWeb]
	}
	if rightAskAccount != nil {
		rightAskPercentage = (rightAskAccount.Free + rightAskAccount.Free + carry.Amount) *
			rightAskAccount.PriceInUsdt / ApplicationAccounts.MarketTotal[carry.AskWeb]
	}
	if leftBidAccount != nil {
		leftBidPercentage = (leftBidAccount.Free + leftBidAccount.Frozen + carry.Amount) *
			leftBidAccount.PriceInUsdt / ApplicationAccounts.MarketTotal[carry.BidWeb]
	}
	if rightBidAccount != nil {
		rightBidPercentage = (rightBidAccount.Free + rightBidAccount.Frozen - carry.Amount) *
			rightBidAccount.PriceInUsdt / ApplicationAccounts.MarketTotal[carry.BidWeb]
	}
	if leftAskPercentage >= leftTotalPercentage && rightAskPercentage <= rightTotalPercentage && leftBidPercentage <=
		leftTotalPercentage && rightBidPercentage >= rightTotalPercentage {
		discount := (rightTotalPercentage - rightAskPercentage) / rightTotalPercentage
		if discount < (leftTotalPercentage-leftBidPercentage)/leftTotalPercentage {
			discount = (leftTotalPercentage - leftBidPercentage) / leftTotalPercentage
		}
		// 把将来可能的最低利润减掉作为成本
		reBaseCarryConst := BaseCarryCost - (configMargin-BaseCarryCost)*ApplicationConfig.Deduction
		dynamicMargin := reBaseCarryConst + (configMargin-reBaseCarryConst)*(1-discount)
		if dynamicMargin < BaseCarryCost {
			dynamicMargin = BaseCarryCost
		}
		util.Notice(fmt.Sprintf("%s -> %s %s discount:%f实际门槛 %f", carry.AskWeb, carry.BidWeb,
			carry.Symbol, discount, dynamicMargin))
		return dynamicMargin
	}
	return configMargin
}

func (carry *Carry) CheckWorthSaveMargin() bool {
	if BaseCarryCost < (carry.AskPrice-carry.BidPrice)/carry.AskPrice {
		return true
	}
	return false
}
func (carry *Carry) CheckWorthAmount() bool {
	currencies := strings.Split(carry.Symbol, `_`)
	if len(currencies) == 2 {
		minAmount := ApplicationConfig.MinNum[currencies[0]]
		if carry.Amount >= minAmount {
			return true
		}
	}
	return false
}

func (carry *Carry) CheckWorthCarryMargin(markets *Markets, config *Config) (bool, error) {
	configMargin, _ := config.GetMargin(carry.Symbol)
	dynamicMargin := getDynamicMargin(carry, configMargin)
	carry.Margin = dynamicMargin
	margin := carry.AskPrice - carry.BidPrice
	if margin > 0 && margin > carry.AskPrice*dynamicMargin && carry.Amount > 0 {
		util.Notice(fmt.Sprintf("利润门槛:%.4f 值得搬砖%s", dynamicMargin, carry.ToString()))
		return true, nil
	}
	util.Notice("利润不足" + carry.ToString())
	return false, errors.New("利润不足")
}

func (carry *Carry) CheckWorthCarryTime(markets *Markets, config *Config) (bool, error) {
	now := util.GetNowUnixMillion()
	bidTimeDelay := math.Abs(float64(now - carry.BidTime))
	askTimeDelay := math.Abs(float64(now - carry.AskTime))
	timeDiff := math.Abs(float64(carry.BidTime - carry.AskTime))
	delay, _ := config.GetDelay(carry.Symbol)
	if timeDiff > delay || bidTimeDelay > delay || askTimeDelay > delay {
		message := strconv.Itoa(int(now)) + "时间问题，卖方" + carry.AskWeb + strconv.Itoa(int(carry.AskTime)) + "隔" + strconv.Itoa(int(askTimeDelay))
		message = message + "买方" + carry.BidWeb + strconv.Itoa(int(carry.BidTime)) + "隔" + strconv.Itoa(int(bidTimeDelay))
		util.Notice(message)
		return false, errors.New(message)
	}
	return true, nil
}

func (carry *Carry) ToString() string {
	return fmt.Sprintf("ask间隔 %d bid间隔 %d %s卖%s%.4f时间%d买%s%.4f时间%d数量%f利润%f 利润门槛%f",
		time.Now().Unix()*1000-carry.AskTime, time.Now().Unix()*1000-carry.BidTime, carry.Symbol, carry.AskWeb,
		carry.AskPrice, carry.AskTime, carry.BidWeb, carry.BidPrice, carry.BidTime, carry.Amount,
		(carry.AskPrice-carry.BidPrice)/carry.AskPrice, carry.Margin)
}
