package controller

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"hello/model"
	"hello/util"
	"math/rand"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

var accessTime = make(map[string]int64)
var code = ``

func ParameterServe() {
	router := gin.Default()
	router.GET("/get", GetParameters)
	router.GET("/set", SetParameters)
	router.GET(`/refresh`, RefreshParameters)
	router.GET(`/pw`, GetCode)
	_ = router.Run(":80")
}

func GetCode(c *gin.Context) {
	ip := c.Request.RemoteAddr
	ipTime := accessTime[ip]
	waitTime := (util.GetNowUnixMillion() - ipTime) / 1000
	if waitTime < 30 {
		waitTime = 30 - waitTime
		c.String(http.StatusOK, fmt.Sprintf(`ip %s 还要等待 %d 秒才能再次发送`, ip, waitTime))
	} else {
		accessTime[ip] = util.GetNowUnixMillion()
		rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
		rnd = rand.New(rand.NewSource(rnd.Int63()))
		code = fmt.Sprintf("%06v", rnd.Int31n(1000000))
		auth := smtp.PlainAuth("", "94764906@qq.com", "urszfnsnanxebjga",
			"smtp.qq.com")
		to := []string{model.AppConfig.Mail}
		user := "94764906@qq.com"
		msg := []byte("To: " + strings.Join(to, ",") + "\r\nFrom: 刷单系统<" + user +
			">\r\nSubject: 验证码\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n刷单验证码是" + code)
		err := smtp.SendMail("smtp.qq.com:25", auth, user, to, msg)
		if err != nil {
			fmt.Printf("send mail error: %v", err)
		}
		c.String(http.StatusOK, `发送成功，请查收邮箱`)
		//urszfnsnanxebjga
	}
}

func GetParameters(c *gin.Context) {
	c.String(http.StatusOK, model.AppConfig.ToString())
}

func RefreshParameters(c *gin.Context) {
	model.LoadSettings()
	c.String(http.StatusOK, model.AppConfig.ToString())
}

func SetParameters(c *gin.Context) {
	handle := c.Query("handle")
	if handle == `1` {
		pw := c.Query(`pw`)
		if code == `` {
			c.String(http.StatusOK, `请先获取验证码`)
			return
		}
		if pw != code {
			c.String(http.StatusOK, `验证码错误`)
			return
		}
		ip := c.Request.RemoteAddr
		ipTime := accessTime[ip]
		waitTime := (util.GetNowUnixMillion() - ipTime) / 1000
		if waitTime > 300 {
			c.String(http.StatusOK, `验证码有效时间300秒，已超`)
			return
		}
		code = ``
		model.AppConfig.Handle = 1
	}
	if handle == `0` {
		model.AppConfig.Handle = 0
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
