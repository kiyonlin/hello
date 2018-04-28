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
	if ApplicationAccounts == nil {
		return configMargin
	}
	if !strings.Contains(carry.Symbol, "_") {
		util.SocketInfo(fmt.Sprintf("invalid carry %s %s %f %d", carry.Symbol, carry.BidWeb, carry.Amount, carry.BidTime))
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
	if leftAskPercentage >= leftTotalPercentage && rightAskPercentage <= rightTotalPercentage && leftBidPercentage <=
		leftTotalPercentage && rightBidPercentage >= rightTotalPercentage {
		discount := (rightTotalPercentage - rightAskPercentage) / rightTotalPercentage
		if discount < (leftTotalPercentage-leftBidPercentage)/leftTotalPercentage {
			discount = (leftTotalPercentage - leftBidPercentage) / leftTotalPercentage
		}
		// 把将来可能的最低利润减掉作为成本
		reBaseCarryConst := BaseCarryCost - (configMargin-BaseCarryCost)*0.25
		if reBaseCarryConst < 0 {
			reBaseCarryConst = 0
		}
		util.SocketInfo(fmt.Sprintf("discount:%f实际门槛 %f", discount, reBaseCarryConst+(configMargin-reBaseCarryConst)*(1-discount)))
		return reBaseCarryConst + (configMargin-reBaseCarryConst)*(1-discount)
	}
	return configMargin
}

func (carry *Carry) CheckWorth(markets *Markets, config *Config) (bool, error) {
	if carry == nil {
		return false, errors.New("carry is nil")
	}
	now := util.GetNowUnixMillion()
	bidTimeDelay := math.Abs(float64(now - carry.BidTime))
	askTimeDelay := math.Abs(float64(now - carry.AskTime))
	timeDiff := math.Abs(float64(carry.BidTime - carry.AskTime))
	delay, _ := config.GetDelay(carry.Symbol)
	configMargin, _ := config.GetMargin(carry.Symbol)
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
		util.Notice(fmt.Sprintf("利润门槛:%.4f 值得搬砖%s", dynamicMargin, carry.ToString()))
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
