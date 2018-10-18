package main

import (
	"fmt"
	"github.com/jinzhu/configor"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"hello/api"
	"hello/model"
	"hello/util"
	"sort"
	"testing"
	"time"
)

func Test_RefreshAccount(t *testing.T) {
	model.NewConfig()
	err := configor.Load(model.AppConfig, "./config.yml")
	if err != nil {
		util.Notice(err.Error())
		return
	}
	api.GetAccountOkfuture(model.AppAccounts)
	for i := 0; i < 50; i++ {
		api.GetKLineOkexFuture(`btc_this_week`, `1min`, 100)
	}
	timeSlot := `1min`
	size := int64(1560)
	currency := `eos`
	kpointsFuture := api.GetKLineOkexFuture(currency+`_this_week`, timeSlot, size)
	kpoints := api.GetKLineOkex(currency+`_usdt`, timeSlot, size)
	percentage := 0.0
	ticks := make([]model.Tick, len(kpointsFuture))
	i := 0
	for key, value := range kpointsFuture {
		futureTime := time.Unix(value.TS/1000, 0).Format("Mon Jan 2 2006-01-02 15:04:05")
		currentTime := time.Unix(kpoints[key].TS/1000, 0).Format("Mon Jan 2 2006-01-02 15:04:05")
		percentage += (value.EndPrice - kpoints[key].EndPrice) / kpoints[key].EndPrice
		tick := model.Tick{Id: futureTime, Price: (value.EndPrice - kpoints[key].EndPrice) / kpoints[key].EndPrice}
		ticks[i] = tick
		i++
		fmt.Println(fmt.Sprintf(`%d: %s - %s = %f `, key, futureTime, currentTime,
			(value.EndPrice-kpoints[key].EndPrice)/kpoints[key].EndPrice))

	}
	var sortedKLine model.Ticks
	sortedKLine = ticks
	percentage = 100 * percentage / float64(size)
	fmt.Println(percentage)
	sort.Sort(sortedKLine)
	for _, value := range sortedKLine {
		fmt.Println(fmt.Sprintf(`%s %f`, value.Id, value.Price))
	}
}
