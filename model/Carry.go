package model

import (
	"fmt"
	"github.com/pkg/errors"
	"hello/util"
	"math"
	"strconv"
	"time"
)

type Carry struct {
	BidSymbol      string
	AskSymbol      string
	BidWeb         string
	AskWeb         string
	BidAmount      float64
	AskAmount      float64
	Amount         float64
	DealBidAmount  float64
	DealAskAmount  float64
	BidPrice       float64
	AskPrice       float64
	DealBidPrice   float64
	DealAskPrice   float64
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

func LoadDiligentSettings(bidWeb, sideType string, createdAt time.Time) (settings map[string]*Setting) {
	settings = make(map[string]*Setting)
	rows, err := AppDB.Model(&Carry{}).Select(`bid_symbol`).Where(
		`bid_web = ? and side_type = ? and created_at > ?`, bidWeb, sideType, createdAt).Group(`bid_symbol`).Rows()
	if err == nil {
		var symbol string
		for rows.Next() {
			rows.Scan(&symbol)
			settings[symbol] = GetSetting(bidWeb, symbol)
		}
	}
	return settings
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
	carry.Margin = GetMargin(carry.BidSymbol)
	margin := carry.AskPrice - carry.BidPrice
	if margin > carry.AskPrice*carry.Margin && carry.Amount > 0 {
		util.Info(fmt.Sprintf("利润门槛:%.4f 值得搬砖%s", carry.Margin, carry.ToString()))
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
	return fmt.Sprintf("%s-%s %s->%s [%s->%s] 间隔%d - %d 数量%f ask %f bid %f 利润%f - %f 价格%f - %f",
		carry.BidSymbol, carry.AskSymbol, carry.BidSymbol, carry.AskSymbol, carry.AskWeb, carry.BidWeb,
		time.Now().Unix()*1000-carry.AskTime, time.Now().Unix()*1000-carry.BidTime, carry.Amount, carry.AskAmount,
		carry.BidAmount, (carry.AskPrice-carry.BidPrice)/carry.AskPrice, carry.Margin, carry.BidPrice, carry.AskPrice)
}
