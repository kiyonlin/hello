package controller

import (
	"github.com/gin-gonic/gin"
	"hello/model"
	"net/http"
	"strconv"
	"strings"
)

func ParameterServe() {
	router := gin.Default()
	router.GET("/get", GetParameters)
	router.GET("/set", SetParameters)
	router.Run(":8080")
}

func GetParameters(c *gin.Context) {
	c.String(http.StatusOK, model.ApplicationConfig.ToString())
}

func SetParameters(c *gin.Context) {
	pw := c.Query(`pw`)
	if pw != `1234qwer` {
		c.String(http.StatusOK, `密码错误`)
		return
	}
	handle := c.Query("handle")
	if handle != `` {
		model.ApplicationConfig.Handle, _ = strconv.ParseInt(handle, 10, 64)
	}
	orderWait := c.Query("orderwait")
	if orderWait != `` {
		model.ApplicationConfig.OrderWait, _ = strconv.ParseInt(orderWait, 10, 64)
	}
	amountRate := c.Query("amountrate")
	if amountRate != `` {
		model.ApplicationConfig.AmountRate, _ = strconv.ParseFloat(amountRate, 64)
	}
	sellRate := c.Query(`sellrate`)
	if sellRate != `` {
		model.ApplicationConfig.SellRate, _ = strconv.ParseFloat(sellRate, 64)
	}
	ftMax := c.Query(`ftmax`)
	if ftMax != `` {
		model.ApplicationConfig.FtMax, _ = strconv.ParseFloat(ftMax, 64)
	}
	deduction := c.Query("deduction")
	if len(strings.TrimSpace(deduction)) > 0 {
		value, _ := strconv.ParseFloat(deduction, 64)
		if value > 0 && value < 1 {
			model.ApplicationConfig.Deduction = value
		}
	}
	channelSlot := c.Query("channelslot")
	if len(strings.TrimSpace(channelSlot)) > 0 {
		value, _  :=  strconv.ParseFloat(channelSlot, 64)
		if value > 0 {
			model.ApplicationConfig.ChannelSlot = value
		}
	}
	channels := c.Query("channels")
	if len(strings.TrimSpace(channels)) > 0 {
		temp, _ := strconv.ParseInt(channels, 10, 64)
		model.ApplicationConfig.Channels = int(temp)
	}
	minUsdt := c.Query("minusdt")
	if len(strings.TrimSpace(minUsdt)) > 0 {
		model.ApplicationConfig.MinUsdt, _ = strconv.ParseFloat(minUsdt, 64)
	}
	maxUsdt := c.Query("maxusdt")
	if len(strings.TrimSpace(maxUsdt)) > 0 {
		model.ApplicationConfig.MaxUsdt, _ = strconv.ParseFloat(maxUsdt, 64)
	}
	env := c.Query("env")
	if len(strings.TrimSpace(env)) > 0 {
		model.ApplicationConfig.Env = env
	}
	markets := c.Query("markets")
	if len(strings.TrimSpace(markets)) > 0 {
		model.ApplicationConfig.Markets = strings.Split(strings.Replace(markets, " ", "", -1), ",")
	}
	symbols := c.Query("symbols")
	if len(strings.TrimSpace(symbols)) > 0 {
		model.ApplicationConfig.Symbols = strings.Split(strings.Replace(symbols, " ", "", -1), ",")
	}
	margins := c.Query("margins")
	if len(strings.TrimSpace(margins)) > 0 {
		strMargins := strings.Split(strings.Replace(margins, " ", "", -1), ",")
		model.ApplicationConfig.Margins = make([]float64, len(strMargins))
		for key, value := range strMargins {
			floatValue, _ := strconv.ParseFloat(value, 64)
			if floatValue > 0 {
				model.ApplicationConfig.Margins[key] = floatValue
			}
		}
	}
	delays := c.Query("delays")
	if len(strings.TrimSpace(delays)) > 0 {
		strDelays := strings.Split(strings.Replace(delays, " ", "", -1), ",")
		model.ApplicationConfig.Delays = make([]float64, len(strDelays))
		for key, value := range strDelays {
			model.ApplicationConfig.Delays[key], _ = strconv.ParseFloat(value, 64)
		}
	}
	c.String(http.StatusOK, model.ApplicationConfig.ToString())
}
