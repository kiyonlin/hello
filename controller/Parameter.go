package controller

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"hello/api"
	"hello/carry"
	"hello/model"
	"hello/util"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//var accessTime = make(map[string]int64)
var codeGenTime int64
var code = ``
var data = make(map[string]interface{})
var dataUpdateTime *time.Time
var balances = make(map[string]map[string]map[string][]float64) // time - market - currency - value

func ParameterServe() {
	router := gin.Default()
	router.LoadHTMLGlob("templates/*")
	router.GET("/get", GetParameters)
	router.GET("/set", SetParameters)
	router.GET(`/refresh`, RefreshParameters)
	router.GET(`/pw`, GetCode)
	router.GET("/balance", GetBalance)
	router.GET(`/symbol`, setSymbol)
	router.GET(`/test`, test)
	_ = router.Run(":" + model.AppConfig.Port)
}

func test(c *gin.Context) {
	util.SocketInfo(fmt.Sprintf(`get test request %d`, util.GetNowUnixMillion()))
	c.String(http.StatusOK, `thank you ray`)
}

func setSymbol(c *gin.Context) {
	pw := c.Query(`pw`)
	if code == `` {
		c.String(http.StatusOK, `请先获取验证码`)
		return
	}
	if pw != code {
		c.String(http.StatusOK, `验证码错误`)
		return
	}
	waitTime := (util.GetNowUnixMillion() - codeGenTime) / 1000
	if waitTime > 300 {
		c.String(http.StatusOK, fmt.Sprintf(`验证码有效时间300秒，已超%d - %d > 300000`,
			util.GetNowUnixMillion(), codeGenTime))
		return
	}
	code = ``
	market := c.Query(`market`)
	symbol := c.Query(`symbol`)
	function := c.Query(`function`)
	strLimit := c.Query(`limit`)
	parameter := c.Query(`parameter`)
	binanceDisMin := c.Query(`binancedismin`)
	binanceDisMax := c.Query(`binancedismax`)
	refreshLimitLowStr := c.Query(`refreshlimitlow`)
	refreshLimitStr := c.Query(`refreshlimit`)
	chanceStr := c.Query(`chance`)
	refreshSameTime := c.Query(`refreshsametime`)
	gridAmountStr := c.Query(`gridamount`)
	griddisStr := c.Query(`griddis`)
	priceXStr := c.Query(`pricex`)
	valid := false
	if market == `` || symbol == `` || function == `` {
		c.String(http.StatusOK, `market symbol function cannot be empty`)
	}
	op := c.Query(`op`)
	if op == `1` {
		valid = true
	} else if op == `0` {
		valid = false
	}
	var setting model.Setting
	amountLimit := 0.0
	model.AppDB.Model(&setting).Where("function_parameter is null").Update("function_parameter", ``)
	model.AppDB.Model(&setting).Where("account_type is null").Update("account_type", ``)
	model.AppDB.Model(&setting).Where("amount_limit is null").Update("amount_limit", ``)
	if op != `` {
		model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
			market, symbol, function).Updates(map[string]interface{}{"valid": valid})
	}
	if parameter != `` {
		model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
			market, symbol, function).Updates(map[string]interface{}{`function_parameter`: parameter})
	}
	if priceXStr != `` {
		priceX, _ := strconv.ParseFloat(priceXStr, 64)
		model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
			market, symbol, function).Updates(map[string]interface{}{`price_x`: priceX})
	}
	if gridAmountStr != `` {
		gridAmount, _ := strconv.ParseFloat(gridAmountStr, 64)
		model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
			market, symbol, function).Updates(map[string]interface{}{`grid_amount`: gridAmount})
	}
	if griddisStr != `` {
		gridPriceDistance, _ := strconv.ParseFloat(griddisStr, 64)
		model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
			market, symbol, function).Updates(map[string]interface{}{`grid_price_distance`: gridPriceDistance})
	}
	if strLimit != `` {
		amountLimit, _ = strconv.ParseFloat(strLimit, 64)
		model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
			market, symbol, function).Updates(map[string]interface{}{`amount_limit`: amountLimit})
	}
	if binanceDisMin != `` {
		bDisMin, _ := strconv.ParseFloat(binanceDisMin, 64)
		model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
			market, symbol, function).Updates(map[string]interface{}{`binance_dis_min`: bDisMin})
	}
	if binanceDisMax != `` {
		bDisMax, _ := strconv.ParseFloat(binanceDisMax, 64)
		model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
			market, symbol, function).Updates(map[string]interface{}{`binance_dis_max`: bDisMax})
	}
	if refreshLimitLowStr != `` {
		refreshLimitLow, _ := strconv.ParseFloat(refreshLimitLowStr, 64)
		model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
			market, symbol, function).Updates(map[string]interface{}{`refresh_limit_low`: refreshLimitLow})
	}
	if refreshSameTime != `` {
		value, _ := strconv.ParseFloat(refreshSameTime, 64)
		model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
			market, symbol, function).UpdateColumn("refresh_same_time", value)
	}
	if refreshLimitStr != `` {
		refreshLimit, _ := strconv.ParseFloat(refreshLimitStr, 64)
		model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
			market, symbol, function).Updates(map[string]interface{}{`refresh_limit`: refreshLimit})
	}
	if chanceStr != `` {
		chance, _ := strconv.ParseFloat(chanceStr, 64)
		model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
			market, symbol, function).UpdateColumn("chance", chance)
	}
	rows, _ := model.AppDB.Model(&setting).
		Select(`market, symbol, function, function_parameter, amount_limit, refresh_same_time, valid`).Rows()
	defer rows.Close()
	msg := ``
	for rows.Next() {
		_ = rows.Scan(&market, &symbol, &function, &parameter, &amountLimit, &refreshSameTime, &valid)
		msg += fmt.Sprintf("%s %s %s %s %f %s\n", market, symbol, function, parameter, amountLimit,
			refreshSameTime)
	}
	model.LoadSettings()
	carry.MaintainMarketChan()
	c.String(http.StatusOK, msg)
}

func GetCode(c *gin.Context) {
	waitTime := (util.GetNowUnixMillion() - codeGenTime) / 1000
	if waitTime < 30 {
		waitTime = 30 - waitTime
		c.String(http.StatusOK, fmt.Sprintf(`还要等待 %d 秒才能再次发送`, waitTime))
	} else {
		codeGenTime = util.GetNowUnixMillion()
		rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
		rnd = rand.New(rand.NewSource(rnd.Int63()))
		code = fmt.Sprintf("%06v", rnd.Int31n(1000000))
		err := util.SendMail(model.AppConfig.Mail, `启动验证码`, `验证码是`+code)
		//auth := smtp.PlainAuth("", "94764906@qq.com", "urszfnsnanxebjga",
		//	"smtp.qq.com")
		//msg := []byte("To: " + strings.Join(to, ",") + "\r\nFrom: 刷单系统<" + user +
		//	">\r\nSubject: 验证码\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n刷单验证码是" + code)
		//err := smtp.SendMail("smtp.qq.com:465", auth, user, to, msg)
		if err == nil {
			c.String(http.StatusOK, `发送成功，请查收邮箱`)
		} else {
			c.String(http.StatusOK, `邮件发送失败`+err.Error())
		}
		// iamp pjmyzgvrlifpcbci
	}
}

func renderOrder() {
	d, _ := time.ParseDuration("-240h")
	timeLine := util.GetNow().Add(d)
	rows, _ := model.AppDB.Table("orders").Select(`symbol, date(created_at), count(id), order_side, 
		round(sum(deal_amount*deal_price),4), round(sum(deal_amount*deal_price)/sum(deal_amount),8)`).
		Where(`date(created_at) > ? and function = ?`, timeLine, `grid`).
		Group(`symbol, date(created_at), order_side`).Having(`sum(deal_amount)>?`, 0).
		Order(`date(created_at) desc`).Rows()
	defer rows.Close()
	orderTimes := make([]string, 0)
	orders := make(map[string]map[string]map[string][]float64) // date - symbol - orderSide - [count, amount, price]
	for rows.Next() {
		var symbol, orderSide string
		var count, amount, price float64
		var date time.Time
		_ = rows.Scan(&symbol, &date, &count, &orderSide, &amount, &price)
		dateStr := date.Format("01-02")
		if len(orderTimes) == 0 || orderTimes[len(orderTimes)-1] != dateStr {
			orderTimes = append(orderTimes, dateStr)
		}
		if orders[dateStr] == nil {
			orders[dateStr] = make(map[string]map[string][]float64)
		}
		if orders[dateStr][symbol] == nil {
			orders[dateStr][symbol] = make(map[string][]float64)
		}
		if orders[dateStr][symbol][orderSide] == nil {
			orders[dateStr][symbol][orderSide] = make([]float64, 3)
		}
		orders[dateStr][symbol][orderSide][0] = count
		currencies := strings.Split(symbol, `_`)
		if len(currencies) > 0 {
			priceUsdt, _ := api.GetPrice(``, ``, currencies[1]+`_usdt`)
			amount = math.Round(priceUsdt * amount)
			price = math.Round(priceUsdt*price*1000) / 1000
		}
		orders[dateStr][symbol][orderSide][1] = amount
		orders[dateStr][symbol][orderSide][2] = price
	}
	data[`orders`] = orders
	data[`orderTimes`] = orderTimes
}

func renderBalance() {
	d, _ := time.ParseDuration("-240h")
	timeLine := util.GetNow().Add(d)
	rows, _ := model.AppDB.Table("accounts").Select(`timestamp, market, currency, round(price_in_usdt,2),
		round(free*price_in_usdt, 0),round(frozen*price_in_usdt,0)`).Where(`timestamp > ?`, timeLine).
		Order(`timestamp desc`).Rows()
	defer rows.Close()
	times := make([]string, 0)
	inAlls := make(map[string]float64)
	for rows.Next() {
		var date time.Time
		var market, currency string
		var price, free, froze float64
		_ = rows.Scan(&date, &market, &currency, &price, &free, &froze)
		if dataUpdateTime == nil || dataUpdateTime.Before(date) {
			dataUpdateTime = &date
		}
		if free < 5 && froze < 5 {
			continue
		}
		dateStr := date.Format("01-02 15:04")
		if len(times) == 0 || times[len(times)-1] != dateStr {
			times = append(times, dateStr)
		}
		if balances[dateStr] == nil {
			balances[dateStr] = make(map[string]map[string][]float64)
		}
		if balances[dateStr][market] == nil {
			balances[dateStr][market] = make(map[string][]float64)
		}
		if balances[dateStr][market][currency] == nil {
			balances[dateStr][market][currency] = make([]float64, 4)
		}
		balances[dateStr][market][currency][0] = free
		balances[dateStr][market][currency][1] = froze
		balances[dateStr][market][currency][2] = price
		inAlls[dateStr] += free + froze
		for key := range balances[dateStr][market] {
			balances[dateStr][market][key][3] = math.Round(
				(balances[dateStr][market][key][0] + balances[dateStr][market][key][1]) / inAlls[dateStr] * 100)
		}
	}
	data[`times`] = times
	data[`balances`] = balances
	data[`inAlls`] = inAlls
}

func GetBalance(c *gin.Context) {
	d, _ := time.ParseDuration("1h")
	timeLine := util.GetNow().Add(d)
	if dataUpdateTime != nil && timeLine.Before(*dataUpdateTime) {
		c.HTML(http.StatusOK, "balance.html", data)
		return
	}
	renderBalance()
	renderOrder()
	c.HTML(http.StatusOK, "balance.html", data)
}

func GetParameters(c *gin.Context) {
	var setting model.Setting
	rows, _ := model.AppDB.Model(&setting).Select(`market, symbol, function, grid_amount, grid_price_distance, 
		function_parameter,amount_limit,refresh_limit_low, refresh_limit, valid, price_x`).Rows()
	zb := api.GetFundingRate(model.Bitmex, `btcusd_p`)
	zf := api.GetFundingRate(model.Fmex, `btcusd_p`)
	msg := fmt.Sprintf(`资金费率zb:%f zf:%f\n`, zb, zf)
	for rows.Next() {
		_ = rows.Scan(&setting)
		var market, symbol, function, parameter, amountLimit, refreshLimitLow, refreshLimit, gridAmount,
			gridPriceDistance, priceX string
		valid := false
		_ = rows.Scan(&market, &symbol, &function, &gridAmount, &gridPriceDistance, &parameter, &amountLimit,
			&refreshLimitLow, &refreshLimit, &valid, &priceX)
		msg += fmt.Sprintf("%s %s %s parameter:%s A总:%s 下单数量:%s 价差：%s refreshlimitlow:%s "+
			"refreshlimit:%s %v priceX:%s\n",
			market, symbol, function, parameter, amountLimit, gridAmount, gridPriceDistance, refreshLimitLow,
			refreshLimit, valid, priceX)
	}
	rows.Close()
	msg += model.AppConfig.ToString()
	c.String(http.StatusOK, msg)
}

func RefreshParameters(c *gin.Context) {
	model.LoadSettings()
	for _, market := range model.GetMarkets() {
		channel := model.AppMarkets.GetDepthChan(market, 0)
		if channel != nil {
			carry.ResetChannel(market, channel)
		}
	}
	c.String(http.StatusOK, model.AppConfig.ToString())
}

func SetParameters(c *gin.Context) {
	handle := c.Query("handle")
	handleMaker := c.Query(`handlemaker`)
	handleRefresh := c.Query(`handlerefresh`)
	handleGrid := c.Query(`handlegrid`)
	if len(handle) > 0 || handleMaker == `1` || handleRefresh == `1` || handleGrid == `1` {
		pw := c.Query(`pw`)
		if code == `` {
			c.String(http.StatusOK, `请先获取验证码`)
			return
		}
		if pw != code {
			c.String(http.StatusOK, `验证码错误`)
			return
		}
		waitTime := (util.GetNowUnixMillion() - codeGenTime) / 1000
		if waitTime > 300 {
			c.String(http.StatusOK, fmt.Sprintf(`验证码有效时间300秒，已超%d - %d > 300000`,
				util.GetNowUnixMillion(), codeGenTime))
			return
		}
		code = ``
	}
	if handle != `` {
		model.AppConfig.Handle = handle
	}
	if handleMaker != `` {
		model.AppConfig.HandleMaker = handleMaker
	}
	if handleRefresh != `` {
		model.AppConfig.HandleRefresh = handleRefresh
	}
	if handleGrid != `` {
		model.AppConfig.HandleGrid = handleGrid
	}
	between := c.Query(`between`)
	if len(between) > 0 {
		model.AppConfig.Between, _ = strconv.ParseInt(between, 10, 64)
	}
	predealdis := c.Query(`predealdis`)
	if len(predealdis) > 0 {
		model.AppConfig.PreDealDis, _ = strconv.ParseFloat(predealdis, 64)
	}
	binanceOrderDis := c.Query(`binanceorderdis`)
	if len(binanceOrderDis) > 0 {
		model.AppConfig.BinanceOrderDis, _ = strconv.ParseFloat(binanceOrderDis, 64)
	}
	orderWait := c.Query("orderwait")
	if orderWait != `` {
		model.AppConfig.OrderWait, _ = strconv.ParseInt(orderWait, 10, 64)
	}
	amountRate := c.Query("amountrate")
	if amountRate != `` {
		model.AppConfig.AmountRate, _ = strconv.ParseFloat(amountRate, 64)
	}
	carryDistance := c.Query("carrydistance")
	if len(strings.TrimSpace(carryDistance)) > 0 {
		value, _ := strconv.ParseFloat(carryDistance, 64)
		if value > 0 && value < 1 {
			model.AppConfig.CarryDistance = value
		}
	}
	channelSlot := c.Query("channelslot")
	if len(strings.TrimSpace(channelSlot)) > 0 {
		value, _ := strconv.ParseFloat(channelSlot, 64)
		if value > 0 {
			model.AppConfig.ChannelSlot = value
		}
	}
	channels := c.Query("channels")
	if len(strings.TrimSpace(channels)) > 0 {
		temp, _ := strconv.ParseInt(channels, 10, 64)
		model.AppConfig.Channels = int(temp)
	}
	minUsdt := c.Query("minusdt")
	if len(strings.TrimSpace(minUsdt)) > 0 {
		model.AppConfig.MinUsdt, _ = strconv.ParseFloat(minUsdt, 64)
	}
	maxUsdt := c.Query("maxusdt")
	if len(strings.TrimSpace(maxUsdt)) > 0 {
		model.AppConfig.MaxUsdt, _ = strconv.ParseFloat(maxUsdt, 64)
	}
	delay := c.Query("delay")
	if len(strings.TrimSpace(delay)) > 0 {
		strDelay := strings.Replace(delay, " ", "", -1)
		model.AppConfig.Delay, _ = strconv.ParseFloat(strDelay, 64)
	}
	c.String(http.StatusOK, model.AppConfig.ToString())
}
