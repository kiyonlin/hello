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
	// time_idx的设计有一定的冲突风险，但为了在发起挂单前减少一次db操作而不使用carry的id
	BidTime   int64 `gorm:"unique_index:time_idx;"`
	AskTime   int64 `gorm:"unique_index:time_idx;"`
	ID        uint  `gorm:"primary_key"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func getDynamicMargin(carry *Carry, configMargin float64) (dynamicMargin float64) {
	currencies := strings.Split(carry.Symbol, "_")
	var leftAskPercentage, rightAskPercentage, leftBidPercentage, rightBidPercentage float64
	leftAskAccount := ApplicationAccounts.GetAccount(carry.AskWeb, currencies[0])
	rightAskAccount := ApplicationAccounts.GetAccount(carry.AskWeb, currencies[1])
	leftBidAccount := ApplicationAccounts.GetAccount(carry.BidWeb, currencies[0])
	rightBidAccount := ApplicationAccounts.GetAccount(carry.BidWeb, currencies[1])
	if leftAskAccount != nil {
		leftAskPercentage = leftAskAccount.Percentage
	}
	if rightAskAccount != nil {
		rightAskPercentage = rightAskAccount.Percentage
	}
	if leftBidAccount != nil {
		leftBidPercentage = leftBidAccount.Percentage
	}
	if rightBidAccount != nil {
		rightBidPercentage = rightBidAccount.Percentage
	}
	leftAskBalance := ApplicationConfig.Markets[carry.AskWeb][currencies[0]]
	rightAskBalance := ApplicationConfig.Markets[carry.AskWeb][currencies[1]]
	leftBidBalance := ApplicationConfig.Markets[carry.BidWeb][currencies[0]]
	rightBidBalance := ApplicationConfig.Markets[carry.BidWeb][currencies[1]]
	if leftAskPercentage >= leftAskBalance && rightAskPercentage <=
		rightAskBalance && leftBidPercentage <= leftBidBalance && rightBidPercentage >= rightBidBalance {
		discount := (rightAskBalance - rightAskPercentage) / rightAskBalance
		if discount < (leftBidBalance-leftBidPercentage)/leftBidBalance {
			discount = (leftBidBalance - leftBidPercentage) / leftBidBalance
		}
		util.SocketInfo(fmt.Sprintf("执行动态搬砖利润折扣%.4f", discount))
		return BaseCarryCost + (configMargin-BaseCarryCost)*discount
	}
	return configMargin
}

func (carry *Carry) CheckWorth(markets *Markets, config *Config, symbol string) (bool, error) {
	if carry == nil {
		return false, errors.New("carry is nil")
	}
	now := util.GetNowUnixMillion()
	bidTimeDelay := math.Abs(float64(now - carry.BidTime))
	askTimeDelay := math.Abs(float64(now - carry.AskTime))
	timeDiff := math.Abs(float64(carry.BidTime - carry.AskTime))
	delay, _ := config.GetDelay(symbol)
	configMargin, _ := config.GetMargin(symbol)
	dynamicMargin := getDynamicMargin(carry, configMargin)
	if timeDiff > delay || bidTimeDelay > delay || askTimeDelay > delay {
		message := strconv.Itoa(int(now)) + "时间问题，卖方" + carry.AskWeb + strconv.Itoa(int(carry.AskTime)) + "隔" + strconv.Itoa(int(askTimeDelay))
		message = message + "买方" + carry.BidWeb + strconv.Itoa(int(carry.BidTime)) + "隔" + strconv.Itoa(int(bidTimeDelay))
		util.Info(message)
		return false, errors.New(message)
	}
	margin := carry.AskPrice - carry.BidPrice
	if margin > 0 && margin > carry.AskPrice*dynamicMargin && carry.Amount > 0 {
		if carry.BidAmount > carry.AskAmount {
			carry.Amount = carry.AskAmount
		} else {
			carry.Amount = carry.BidAmount
		}
		util.Info(carry.ToString())
		return true, nil
	}
	util.Info("利润不足" + carry.ToString())
	return false, errors.New("利润不足")
}

func (carry *Carry) ToString() string {
	bidPrice := strconv.FormatFloat(carry.BidPrice, 'f', -1, 64)
	askPrice := strconv.FormatFloat(carry.AskPrice, 'f', -1, 64)
	margin := strconv.FormatFloat(100*(carry.AskPrice-carry.BidPrice)/carry.AskPrice, 'f', -1, 64)
	amount := strconv.FormatFloat(carry.Amount, 'f', -1, 64)
	str := carry.Symbol + "卖:" + carry.AskWeb + askPrice + "时间" + strconv.Itoa(int(carry.AskTime)) + "买"
	str += carry.BidWeb + bidPrice + "时间" + strconv.Itoa(int(carry.BidTime)) + "数量:" + amount + "利润:" + margin + "%"
	return str
}
