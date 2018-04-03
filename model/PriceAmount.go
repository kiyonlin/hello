package model

type PriceAmount [][]float64

func (priceAmount PriceAmount) Len() int {
	return len(priceAmount)
}

func (priceAmount PriceAmount) Swap(i, j int) {
	priceAmount[i], priceAmount[j] = priceAmount[j], priceAmount[i]
}

func (priceAmount PriceAmount) Less(i, j int) bool {
	return priceAmount[i][0] < priceAmount[j][0]
}
