package main

import (
	"math"

	"gonum.org/v1/gonum/stat"
)

func truncatePrec(n float64) float64 {
	// e := math.Pow(10, float64(precision)) // 10 в степень precision
	return float64(int(n*100+math.Copysign(0.5, n))) / 100
}

// func addSupsToItems(items []*stat_products_item, sups []stat_products_item_sup) {
// 	for _, item := range items {
// 		item.Suppliers_stat = make([]stat_products_item_sup, len(sups))
// 		copy(item.Suppliers_stat, sups)
// 	}
// }

func (item *stat_products_item) calcStats() {
	for i := range item.Suppliers_stat {
		if len(item.Suppliers_stat[i].stocks_nozeroes) > 0 {
			item.Suppliers_stat[i].Stock_avg = truncatePrec(stat.Mean(item.Suppliers_stat[i].stocks, nil))
			item.Suppliers_stat[i].Stock_avg_nozeroes = truncatePrec(stat.Mean(item.Suppliers_stat[i].stocks_nozeroes, nil))
			item.Suppliers_stat[i].Price_avg = truncatePrec(stat.Mean(item.Suppliers_stat[i].prices, nil))
			item.Suppliers_stat[i].Stock_dispersion = truncatePrec(stat.PopVariance(item.Suppliers_stat[i].stocks, nil))
		}
	}
}
