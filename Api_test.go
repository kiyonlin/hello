package main

import (
	"fmt"
	"github.com/jinzhu/configor"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"hello/api"
	"hello/carry"
	"hello/model"
	"hello/util"
	"testing"
	"time"
)

func Test_chan(t *testing.T) {
	model.NewConfig()
	var err error
	model.AppDB, err = gorm.Open("postgres", model.AppConfig.DBConnection)
	if err != nil {
		util.Notice(err.Error())
		return
	}
	defer model.AppDB.Close()
	carry.MaintainMarketChan()
}

func Test_initTurtleN(t *testing.T) {
	model.NewConfig()
	_ = configor.Load(model.AppConfig, "./config.yml")
	var err error
	model.AppDB, err = gorm.Open("postgres", model.AppConfig.DBConnection)
	if err != nil {
		util.Notice(err.Error())
		return
	}
	model.AppDB.AutoMigrate(&model.Candle{})
	today := time.Now().In(time.UTC)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	//api.GetDayCandle(`HHCJIVMpxYEahfxGZH9NoFzD`, `T9PD2va1ovmiiZroFIqJnKL_k6ZLGC3hkay-hKrPiOROe_MY`,
	//	model.Bitmex, `btcusd_p`, yesterday)
	fmt.Println(today.String())
	for i := 100; i > 0; i-- {
		d, _ := time.ParseDuration(fmt.Sprintf(`%dh`, -24*i))
		index := today.Add(d)
		fmt.Println(index.String())
		api.GetDayCandle(`HHCJIVMpxYEahfxGZH9NoFzD`, `T9PD2va1ovmiiZroFIqJnKL_k6ZLGC3hkay-hKrPiOROe_MY`,
			model.Bitmex, `btcusd_p`, index)
	}
	//go carry.CheckPastRefresh()
	//for true {
	//	time.Sleep(time.Minute)
	//}
	//d, _ := time.ParseDuration("-24h")
	//timeLine := util.GetNow().Add(d)
	//before := util.GetNow().Unix()
	//after := timeLine.Unix()
	//orders := api.QueryOrders(model.Fcoin, `eos_usdt`, model.CarryStatusWorking, before, after)
	//for _, order := range orders {
	//	if order != nil && order.OrderId != `` {
	//		//result, errCode, msg := api.CancelOrder(market, symbol, order.OrderId)
	//		util.Notice(fmt.Sprintf(`[cancel old]%v %s %f`, true, order.OrderId, order.Price))
	//		time.Sleep(time.Millisecond * 100)
	//	}
	//}
	//api.QueryOrderDealsFcoin(`3BgqYy6o70gMlDiCgH0JJEEynoJPqYnz5SZSq-No0EhA2-D4pKe6BB0RqdfJ0fXTDCfKUfhBVHyAFphKAWwylA==`)
	//orders := api.QueryOrders(model.Fcoin, `btc_usdt`, `success`,
	//	1557529200, 1557504000)
	//for _, value := range orders {
	//	util.Notice(fmt.Sprintf(`,symbol:%s,%s,%s,%s,%s,%f,%f,%f,%f`,
	//		value.Symbol, value.OrderTime.String(), value.Function, value.OrderSide, value.Status,
	//		value.DealAmount, value.DealPrice, value.Fee, value.FeeIncome))
	//}
}

func Test_RefreshAccount(t *testing.T) {
	model.NewConfig()
	_ = configor.Load(model.AppConfig, "./config.yml")
	//rate, ts := api.GetFundingRate(model.Ftx, `btcusd_p`)
	//fmt.Println(fmt.Sprintf(`%f %d`, rate, ts))
	//api.RefreshAccount(``, ``, model.Ftx)
	//api.CreateSubAccount(model.AppConfig.FtxKey, model.AppConfig.FtxSecret)
	order := api.PlaceOrder(model.AppConfig.FtxKey, model.AppConfig.FtxSecret,
		model.OrderSideSell, model.OrderTypeLimit,
		model.Ftx, `xrpusd_p`, ``, ``, ``, 0.235567,
		1, false)
	fmt.Println(order.OrderId)
	//api.RefreshAccount(model.AppConfig.OkexKey, model.AppConfig.OkexSecret, model.OKSwap)
	//api.PlaceOrder(model.AppConfig.OkexKey, model.AppConfig.OkexSecret, model.OrderSideBuy, ``, model.OKSwap,
	//	`btcusd_p`, ``, ``, ``, 9666, 300, false)
	//api.QueryOrderById(model.AppConfig.BitmexKey, model.AppConfig.BitmexSecret, model.Bitmex, `ethusd_p`,
	//	`44202b24-36e5-d07e-bc6d-b717b4f198f1`)
	//api.CancelOrder(model.AppConfig.BitmexKey, model.AppConfig.BitmexSecret, model.OKSwap, `btcusd_p`, `e7a8248a-ac13-fc5f-245c-496ca7a816b0`)
}

func Test_wallet(t *testing.T) {
	model.NewConfig()
	_ = configor.Load(model.AppConfig, "./config.yml")
	//amount, transfer := api.GetWalletHistoryBitmex(model.AppConfig.BitmexKey, model.AppConfig.BitmexSecret)
	//fmt.Println(fmt.Sprintf("%f \n%s", amount, transfer))
	fmt.Println(api.GetWalletBybit(model.AppConfig.BybitKey, model.AppConfig.BybitSecret))
	//balance := api.GetWalletOKSwap(model.AppConfig.OkexKey, model.AppConfig.OkexSecret)
	//for symbol, amount := range balance {
	//	if amount > 0 {
	//		info := api.GetWalletHistoryOKSwap(model.AppConfig.OkexKey, model.AppConfig.OkexSecret, symbol)
	//		fmt.Println(fmt.Sprintf("%s %f\n %s", symbol, amount, info))
	//	}
	//}
}
