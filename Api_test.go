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
	fmt.Println(today.String())
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	symbol := `ETH-USD-200605`
	api.GetDayCandle(``, ``, model.OKFUTURE, symbol, api.GetCurrentInstrument(model.OKFUTURE, symbol), today)
	for i := 100; i > 0; i-- {
		d, _ := time.ParseDuration(fmt.Sprintf(`%dh`, -24*i))
		index := today.Add(d)
		fmt.Println(index.String())
		//api.GetDayCandle(`I9ZmxUz8KsgH6AekmsdQtIdZ33T7bH7SPg_WuBsD`, `WtGav2ou_f9HYUT4B9zj66kig7dJW8t1GEmsgFJp`,
		//	model.Ftx, `ethusd_p`, index)
		//api.GetDayCandle(`I9ZmxUz8KsgH6AekmsdQtIdZ33T7bH7SPg_WuBsD`, `WtGav2ou_f9HYUT4B9zj66kig7dJW8t1GEmsgFJp`,
		//	model.Ftx, `btcusd_p`, index)
		//api.GetDayCandle(`I9ZmxUz8KsgH6AekmsdQtIdZ33T7bH7SPg_WuBsD`, `WtGav2ou_f9HYUT4B9zj66kig7dJW8t1GEmsgFJp`,
		//	model.Ftx, `eosusd_p`, index)
		//api.GetDayCandle(`I9ZmxUz8KsgH6AekmsdQtIdZ33T7bH7SPg_WuBsD`, `WtGav2ou_f9HYUT4B9zj66kig7dJW8t1GEmsgFJp`,
		//	model.Ftx, `htusd_p`, index)
		//api.GetDayCandle(`I9ZmxUz8KsgH6AekmsdQtIdZ33T7bH7SPg_WuBsD`, `WtGav2ou_f9HYUT4B9zj66kig7dJW8t1GEmsgFJp`,
		//	model.Ftx, `bnbusd_p`, index)
		//api.GetDayCandle(`I9ZmxUz8KsgH6AekmsdQtIdZ33T7bH7SPg_WuBsD`, `WtGav2ou_f9HYUT4B9zj66kig7dJW8t1GEmsgFJp`,
		//	model.Ftx, `okbusd_p`, index)
	}
	fmt.Println(`done`)
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
	var err error
	model.AppDB, err = gorm.Open("postgres", model.AppConfig.DBConnection)
	if err != nil {
		util.Notice(err.Error())
		return
	}
	api.QueryOrders(``, ``, model.OKFUTURE, ``, `ETH-USD-200626`, ``, ``, 0, 0)
	order1 := api.PlaceOrder(``, ``, model.OrderSideBuy, model.OrderTypeStop, model.OKFUTURE, `btc-usd`,
		`BTC-USD-200626`, ``, model.AccountTypeLever, ``, ``, 9000.4,
		1, false)
	fmt.Println(order1.OrderId)
	result, _, _, _ := api.CancelOrder(``, ``, model.OKFUTURE, `btc-usd`, `BTC-USD-200626`,
		model.OrderTypeStop, order1.OrderId)
	fmt.Println(result)
	order2 := api.QueryOrderById(``, ``, model.OKFUTURE, `btc-usd`, `BTC-USD-200626`,
		``, `5036936567439361`)
	fmt.Println(order2.OrderId)
	api.RefreshAccount(model.AppConfig.OkexKey, model.AppConfig.OkexSecret, model.OKFUTURE)
	//carry.GetTurtleData(model.Bitmex, `btcusd_p`)
	//rate, ts := api.GetFundingRate(model.Ftx, `btcusd_p`)
	//fmt.Println(fmt.Sprintf(`%f %d`, rate, ts))
	//api.RefreshAccount(``, ``, model.Ftx)
	//api.CreateSubAccount(model.AppConfig.FtxKey, model.AppConfig.FtxSecret)
	//order := api.PlaceOrder(model.AppConfig.FtxKey, model.AppConfig.FtxSecret,
	//	model.OrderSideSell, model.OrderTypeMarket,
	//	model.Bitmex, `btcusd_p`, ``, ``, ``, ``, 5188,
	//	1, false)
	//fmt.Println(order.OrderId)
	//result, _, _, _ := api.CancelOrder(``, ``, model.Ftx, `htusd_p`, model.OrderTypeStop, "899071")
	//fmt.Print(result)
	//api.QueryOrderById("", "", model.Ftx, `htusd_p`, "899071")
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
	model.AppDB, _ = gorm.Open("postgres", model.AppConfig.DBConnection)
	//carry.GetTurtleData(model.Ftx, `okbusd_p`)
	//var err error
	//model.AppDB, err = gorm.Open("postgres", model.AppConfig.DBConnection)
	//if err != nil {
	//	util.Notice(err.Error())
	//	return
	//}
	//today := time.Now().In(time.UTC)
	//today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	//api.GetDayCandle(model.AppConfig.BitmexKey, model.AppConfig.BitmexSecret, model.Bitmex, `btcusd_p`, today)
	//api.GetDayCandle(model.AppConfig.FtxKey, model.AppConfig.FtxSecret, model.Ftx, `htusd_p`, today)
	//balanceUSD := api.GetWalletHistoryFtx(model.AppConfig.FtxKey, model.AppConfig.FtxSecret)
	//balanceUSD := api.GetUSDBalance(model.AppConfig.FtxKey, model.AppConfig.FtxSecret, model.Ftx)
	//fmt.Print(balanceUSD)
	//api.RefreshAccount(model.AppConfig.FtxKey, model.AppConfig.FtxSecret, model.Ftx)
	//order := api.QueryOrderById(model.AppConfig.FtxKey, model.AppConfig.FtxSecret, model.Ftx,
	//	`btcusd_p`, model.OrderTypeStop, `903993`)
	//fmt.Print(order.DealPrice)
	result, _, _, _ := api.CancelOrder(model.AppConfig.BitmexKey, model.AppConfig.BitmexSecret, model.Bitmex,
		`btcusd_p`, ``, model.OrderTypeStop, `fa9a9293-bcd1-4812-501d-c7529b42efed`)
	fmt.Print(result)
	//amount, transfer := api.GetWalletHistoryBitmex(model.AppConfig.BitmexKey, model.AppConfig.BitmexSecret)
	//fmt.Println(fmt.Sprintf("%f \n%s", amount, transfer))
	//fmt.Println(api.GetWalletBybit(model.AppConfig.BybitKey, model.AppConfig.BybitSecret))
	//balance := api.GetWalletOKSwap(model.AppConfig.OkexKey, model.AppConfig.OkexSecret)
	//for symbol, amount := range balance {
	//	if amount > 0 {
	//		info := api.GetWalletHistoryOKSwap(model.AppConfig.OkexKey, model.AppConfig.OkexSecret, symbol)
	//		fmt.Println(fmt.Sprintf("%s %f\n %s", symbol, amount, info))
	//	}
	//}
}
