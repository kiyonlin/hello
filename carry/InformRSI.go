package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"net/smtp"
	"strings"
	"time"
)

var data = make(map[string]map[string][]*model.KLinePoint) // symbol - slot - kline data

func getData(symbol, timeSlot string) []*model.KLinePoint {
	if data[symbol] == nil {
		data[symbol] = make(map[string][]*model.KLinePoint)
	}
	if data[symbol][timeSlot] == nil {
		priceKLine := api.GetKLineOkex(symbol, timeSlot, 2000)
		data[symbol][timeSlot] = priceKLine

		diff := make([]float64, len(data[symbol][timeSlot])-1)
		for i := 0; i < len(diff); i++ {
			diff[i] = data[symbol][timeSlot][i+1].EndPrice - data[symbol][timeSlot][i].EndPrice
		}
		avgUp := 0.0
		avgDown := 0.0
		for i := 5; i < len(diff); i++ {
			up := 0.0
			down := 0.0
			j := i - 5
			for ; j <= 5; j++ {
				if diff[j] > 0 {
					up += diff[j]
				} else {
					down += diff[j]
				}
			}
			if i > 5 {
				if diff[i] > 0 {
					up = diff[i]
				} else {
					down = diff[i]
				}
				avgUp = (avgUp*5 + up) / 6
				avgDown = (avgDown*5 + down) / 6
				data[symbol][timeSlot][i+1].RSI = 100 * avgUp / (avgUp - avgDown)
			} else {
				avgUp = up / 6
				avgDown = down / 6
				data[symbol][timeSlot][i+1].RSI = 100 * avgUp / (avgUp - avgDown)
			}
			max := (data[symbol][timeSlot][i+1].RSI/data[symbol][timeSlot][i].RSI)*5*down - 5*up
			min := 5*up/(data[symbol][timeSlot][i+1].RSI/data[symbol][timeSlot][i].RSI) - 5*down
			data[symbol][timeSlot][i+1].RSIExpectBuy = data[symbol][timeSlot][i].EndPrice - min
			data[symbol][timeSlot][i+1].RSIExpectSell = data[symbol][timeSlot][i+1].EndPrice + max
		}
	}
	return data[symbol][timeSlot]
}

func sendMail(body string) {
	auth := smtp.PlainAuth("", "94764906@qq.com", "urszfnsnanxebjga", "smtp.qq.com")
	to := []string{"haoweizh@qq.com", `ws820714@163.com`}
	nickname := "財神爺"
	user := "haoweizh@qq.com"
	subject := `RSI 15min`
	contentType := "Content-Type: text/plain; charset=UTF-8"
	msg := []byte("To: " + strings.Join(to, ",") + "\r\nFrom: " + nickname +
		"<" + user + ">\r\nSubject: " + subject + "\r\n" + contentType + "\r\n\r\n" + body)
	err := smtp.SendMail("smtp.qq.com:25", auth, user, to, msg)
	if err != nil {
		fmt.Printf("send mail error: %v", err)
	}
}

func ProcessInform() {
	symbols := []string{`btc_usdt`, `eth_usdt`, `eos_usdt`}
	for true {
		if util.GetNow().Minute()%15 != 2 {
			continue
		}
		body := util.GetNow().Format("2006-01-02 15:04:05")
		isSend := false
		for _, symbol := range symbols {
			klines := getData(symbol, `15min`)
			kline := klines[len(klines)-1]
			if kline.RSI < 35 || kline.RSI > 65 {
				isSend = true
			}
			strTime := time.Unix(kline.TS/1000, 0).Format("2006-01-02 15:04:05")
			body = fmt.Sprintf("%s \r\n%s symbol: %s RSI:%f 预计买入价: %f 预计卖出价: %f",
				body, strTime, symbol, kline.RSI, kline.RSIExpectBuy, kline.RSIExpectSell)
		}
		if isSend {
			sendMail(body)
		}
		time.Sleep(time.Minute)
		data = make(map[string]map[string][]*model.KLinePoint)
	}
}
