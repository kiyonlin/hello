package carry

//var ProcessCarry = func(market, symbol string) {
//	if model.AppMarkets.BidAsks[symbol] == nil || model.AppMarkets.BidAsks[symbol][model.Fcoin] == nil ||
//		model.AppMarkets.BidAsks[symbol][model.Huobi] == nil {
//		return
//	}
//	bidFcoin := model.AppMarkets.BidAsks[symbol][model.Fcoin].Bids[0]
//	askFcoin := model.AppMarkets.BidAsks[symbol][model.Fcoin].Asks[0]
//	bidHuobi := model.AppMarkets.BidAsks[symbol][model.Huobi].Bids[0]
//	askHuobi := model.AppMarkets.BidAsks[symbol][model.Huobi].Asks[0]
//	now := util.GetNowUnixMillion()
//	delayHuobi, delayFcoin := now-int64(model.AppMarkets.BidAsks[symbol][model.Huobi].Ts),
//		now-int64(model.AppMarkets.BidAsks[symbol][model.Fcoin].Ts)
//	if delayHuobi > 1500 || delayFcoin > 1500 {
//		util.Notice(fmt.Sprintf(`[dealy too long]%d - %d`, delayHuobi, delayFcoin))
//	}
//
//	carry, err := model.AppMarkets.NewCarry(symbol)
//	if err != nil {
//		util.Notice(`can not create carry ` + err.Error())
//		return
//	}
//	if carry.AskWeb != market && carry.BidWeb != market {
//		util.Notice(`do not create a carry not related to ` + market)
//	}
//	currencies := strings.Split(carry.BidSymbol, "_")
//	leftBalance := 0.0
//	rightBalance := 0.0
//	account := model.AppAccounts.GetAccount(carry.AskWeb, currencies[0])
//	if account == nil {
//		util.Info(`nil account ` + carry.AskWeb + currencies[0])
//		return
//	}
//	leftBalance = account.Free
//	account = model.AppAccounts.GetAccount(carry.BidWeb, currencies[1])
//	if account == nil {
//		util.Info(`nil account ` + carry.BidWeb + currencies[1])
//		return
//	}
//	rightBalance = account.Free
//	priceInUsdt, _ := api.GetPrice(currencies[0] + "_usdt")
//	minAmount := 0.0
//	maxAmount := 0.0
//	if priceInUsdt != 0 {
//		minAmount = model.AppConfig.MinUsdt / priceInUsdt
//		maxAmount = model.AppConfig.MaxUsdt / priceInUsdt
//	}
//	if carry.Amount > maxAmount {
//		carry.Amount = maxAmount
//	}
//	if leftBalance > carry.Amount {
//		leftBalance = carry.Amount
//	}
//	if leftBalance*carry.BidPrice > rightBalance {
//		leftBalance = rightBalance / carry.BidPrice
//	}
//	planAmount, _ := calcAmount(carry.Amount)
//	carry.Amount = planAmount
//	leftBalance, _ = calcAmount(leftBalance)
//	timeOk, _ := carry.CheckWorthCarryTime()
//	marginOk, _ := carry.CheckWorthCarryMargin(model.AppMarkets)
//	if !carry.CheckWorthSaveMargin() {
//		// no need to save carry with margin < base cost
//		return
//	}
//	doCarry := false
//	if !timeOk {
//		carry.DealAskStatus = `NotOnTime`
//		carry.DealBidStatus = `NotOnTime`
//		util.Info(`get carry not on time` + carry.ToString())
//	} else {
//		if !marginOk {
//			carry.DealAskStatus = `NotWorth`
//			carry.DealBidStatus = `NotWorth`
//			util.Info(`get carry no worth` + carry.ToString())
//		} else {
//			model.AppMarkets.BidAsks[carry.AskSymbol][carry.AskWeb] = nil
//			model.AppMarkets.BidAsks[carry.BidSymbol][carry.BidWeb] = nil
//			if leftBalance < minAmount {
//				carry.DealAskStatus = `NotEnough`
//				carry.DealBidStatus = `NotEnough`
//				util.Info(fmt.Sprintf(`leftB %f rightB/bidPrice %f/%f NotEnough %f - %f %s`, account.Free,
//					rightBalance, carry.BidPrice, leftBalance, minAmount, carry.ToString()))
//			} else {
//				util.Notice(`[worth carry]` + carry.ToString())
//				doCarry = true
//			}
//		}
//	}
//	if doCarry {
//		go order(carry, model.OrderSideSell, model.OrderTypeLimit, market, symbol, carry.AskPrice, leftBalance)
//		go order(carry, model.OrderSideBuy, model.OrderTypeLimit, market, symbol, carry.BidPrice, leftBalance)
//	} else {
//		model.CarryChannel <- *carry
//	}
//}
