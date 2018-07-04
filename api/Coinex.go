package api

//Content-Type :`application/json`
//User-Agent: Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36


//func SignedRequestCoinex(method, path string, postMap map[string]interface{}) []byte {
//	hash := hmac.New(md5.New, []byte(model.ApplicationConfig.CoinparkSecret))
//	hash.Write([]byte(cmds))
//	sign := hex.EncodeToString(hash.Sum(nil))
//	postData := &url.Values{}
//	postData.Set("cmds", cmds)
//	postData.Set("apikey", model.ApplicationConfig.CoinparkKey)
//	postData.Set("sign", sign)
//	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded", "User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
//	responseBody, _ := util.HttpRequest(method, model.ApplicationConfig.RestUrls[model.Coinpark]+path,
//		postData.Encode(), headers)
//	return responseBody
//}