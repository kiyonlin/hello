package model

type TurtleStatus struct {
	LastDealPrice float64
	CarryTime     int64
	ExtraBid      float64
	ExtraAsk      float64
}


var appBalanceTurtlePrice = make(map[string]map[string]float64) // market - symbol - last deal price

//var appTurtleStatus map[string]map[string]*TurtleStatus // market - symbol - *TurtleStatus

//func GetTurtleStatus(market, symbol string) (status *TurtleStatus) {
//	if AppTurtleStatus == nil {
//		AppTurtleStatus = make(map[string]map[string]*TurtleStatus)
//	}
//	if AppTurtleStatus[market] == nil {
//		return nil
//	}
//	return AppTurtleStatus[market][symbol]
//}
//
//func SetTurtleStatus(market, symbol string, turtleStatus *TurtleStatus) {
//	if AppTurtleStatus == nil {
//		AppTurtleStatus = make(map[string]map[string]*TurtleStatus)
//	}
//	if AppTurtleStatus[market] == nil {
//		AppTurtleStatus[market] = make(map[string]*TurtleStatus)
//	}
//	turtleStatus.CarryTime = util.GetNowUnixMillion()
//	AppTurtleStatus[market][symbol] = turtleStatus
//}

func SetBalanceTurtlePrice(market, symbol string, price float64) {
	if appBalanceTurtlePrice == nil {
		appBalanceTurtlePrice = make(map[string]map[string]float64)
	}
	if appBalanceTurtlePrice[market] == nil {
		appBalanceTurtlePrice[market] = make(map[string]float64)
	}
	appBalanceTurtlePrice[market][symbol] = price
}

func GetBalanceTurtlePrice(market, symbol string) float64 {
	if appBalanceTurtlePrice == nil {
		return 0
	}
	if appBalanceTurtlePrice[market] == nil {
		return 0
	}
	return appBalanceTurtlePrice[market][symbol]
}
