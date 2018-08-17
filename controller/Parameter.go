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
	router.GET(`/refresh`, RefreshParameters)
	router.Run(":8080")
}

func GetParameters(c *gin.Context) {
	c.String(http.StatusOK, model.AppConfig.ToString())
}

func RefreshParameters(c *gin.Context) {
	model.LoadSettings()
	c.String(http.StatusOK, model.AppConfig.ToString())
}

func SetParameters(c *gin.Context) {
	pw := c.Query(`pw`)
	if pw != `1234qwer` {
		c.String(http.StatusOK, `密码错误`)
		return
	}
	handle := c.Query("handle")
	if handle != `` {
		model.AppConfig.Handle, _ = strconv.ParseInt(handle, 10, 64)
	}
	orderWait := c.Query("orderwait")
	if orderWait != `` {
		model.AppConfig.OrderWait, _ = strconv.ParseInt(orderWait, 10, 64)
	}
	amountRate := c.Query("amountrate")
	if amountRate != `` {
		model.AppConfig.AmountRate, _ = strconv.ParseFloat(amountRate, 64)
	}
	sellRate := c.Query(`sellrate`)
	if sellRate != `` {
		model.AppConfig.SellRate, _ = strconv.ParseFloat(sellRate, 64)
	}
	ftMax := c.Query(`ftmax`)
	if ftMax != `` {
		model.AppConfig.FtMax, _ = strconv.ParseFloat(ftMax, 64)
	}
	deduction := c.Query("deduction")
	if len(strings.TrimSpace(deduction)) > 0 {
		value, _ := strconv.ParseFloat(deduction, 64)
		if value > 0 && value < 1 {
			model.AppConfig.Deduction = value
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
	env := c.Query("env")
	if len(strings.TrimSpace(env)) > 0 {
		model.AppConfig.Env = env
	}
	delay := c.Query("delay")
	if len(strings.TrimSpace(delay)) > 0 {
		strDelay := strings.Replace(delay, " ", "", -1)
		model.AppConfig.Delay, _ = strconv.ParseFloat(strDelay, 64)
	}
	c.String(http.StatusOK, model.AppConfig.ToString())
}
