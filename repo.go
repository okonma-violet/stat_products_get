package main

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDuplicate = errors.New("duplicates")
var ErrNotExists = errors.New("not found")

// const redisDelim = "/_/"

func openPostgresDBRepository(connectionString string) (db *pgxpool.Pool, err error) {
	db, err = pgxpool.New(context.Background(), connectionString)
	return db, err
}

// func (s *service) pingReconPostgres(l logger.Logger) bool {
// 	if err := s.postgresdb.Ping(context.Background()); err != nil {
// 		l.Error("DB/Ping", err)
// 		s.postgresdb.Close(context.Background())
// 		l.Debug("DB", "reconnecting")
// 		var db *pgxpool.Pool
// 		if db, err = openPostgresDBRepository(postgres_connstr); err != nil {
// 			l.Error("DB/OpenDBRepository", err)
// 			return false
// 		}
// 		s.postgresdb = db
// 		l.Info("DB", "reconnected")
// 	}
// 	return true
// }

func getSuppliersSourced(db *pgxpool.Pool, suppliers []int) ([]stat_products_item_sup_stat, []stat_products_item_sup, error) {
	rows, err := db.Query(context.Background(),
		`SELECT s.id,ss.id, s.name from suppliers s JOIN suppliers_sources ss ON s.id=ss.supplierid WHERE s.id=ANY($1)
		ORDER BY s.name ASC, ss.id ASC`, suppliers)
	if err != nil {
		return nil, nil, err
	}
	itemslist := make([]stat_products_item_sup_stat, 0)
	itemslist_s := make([]stat_products_item_sup, 0)
	for rows.Next() {
		s := stat_products_item_sup_stat{
			prices:          make([]float64, 0),
			stocks:          make([]float64, 0),
			stocks_nozeroes: make([]float64, 0),
		}
		if err := rows.Scan(&s.supplierid, &s.sourceid, &s.Supplier_name); err != nil {
			rows.Close()
			return nil, nil, err
		}
		itemslist = append(itemslist, s)
		itemslist_s = append(itemslist_s, stat_products_item_sup{
			Id:   s.supplierid,
			Name: s.Supplier_name,
		})
	}
	if rows.Err() != nil {
		return nil, nil, err
	}
	return itemslist, itemslist_s, nil
}

func (sup *stat_products_item_sup_stat) getUploadsStat(db *pgxpool.Pool, from, to time.Time) error {
	rows, err := db.Query(context.Background(),
		`(SELECT id, time
		FROM uploads_raw
		WHERE sourceid = $1 AND time > $2
		ORDER BY id ASC	LIMIT 1)
		UNION ALL
		(SELECT id, time
		FROM uploads_raw
		WHERE sourceid = $1 AND time < $3
		ORDER BY id DESC LIMIT 1)
		ORDER BY id ASC;`, sup.sourceid, from, to)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int
		var t time.Time
		if err := rows.Scan(&id, &t); err != nil {
			rows.Close()
			return err
		}
		if sup.earliest_upload == 0 {
			sup.earliest_upload = id
		}
		sup.latest_upload = id
	}
	if rows.Err() != nil {
		return err
	}
	return nil
}

func getAllProductsCated(db *pgxpool.Pool, catname string, sups []stat_products_item_sup_stat) ([]*stat_products_item, error) {
	rows, err := db.Query(context.Background(), `SELECT p.brand, p.articul, po.origin_brand, po.origin_articul, po.sourceid, po.stock, po.price, po.name, p.category
	FROM products p JOIN products_offers po ON p.id=po.productid
	WHERE category=$1`, catname)
	if err != nil {
		return nil, err
	}
	items := make([]*stat_products_item, 0)
	itemsidx := make(map[string]map[string]*stat_products_item, 0)

rowing:
	for rows.Next() {
		var brand, articul, orig_br, orig_art, name, cat string
		var srcid, stock int
		var price float64
		if err := rows.Scan(&brand, &articul, &orig_br, &orig_art, &srcid, &stock, &price, &name, &cat); err != nil {
			rows.Close()
			return nil, err
		}

		if ff, ok := itemsidx[brand]; ok {
			if f, ok := ff[articul]; ok {
				for i := range f.Suppliers_stat {
					if f.Suppliers_stat[i].sourceid == srcid {
						if f.Suppliers_stat[i].Brand == "" {
							f.Suppliers_stat[i].Brand = orig_br
							f.Suppliers_stat[i].Articul = orig_art
							f.Suppliers_stat[i].Stock_current = stock
							f.Suppliers_stat[i].Price_current = truncatePrec(price)
							if len(name) > 0 {
								f.namesraw = append(f.namesraw, name)
							}
							break
						} else {
							break
						}
					}
				}
				continue rowing
			}
		} else {
			itemsidx[brand] = make(map[string]*stat_products_item)
		}

		item := stat_products_item{
			Category:       cat,
			Brand:          brand,
			Articul:        articul,
			namesraw:       make([]string, 0),
			Suppliers_stat: make([]stat_products_item_sup_stat, len(sups)),
		}
		copy(item.Suppliers_stat, sups)
		if len(name) > 0 {
			item.namesraw = append(item.namesraw, name)
		}

		for i := range item.Suppliers_stat {
			if item.Suppliers_stat[i].sourceid == srcid {
				item.Suppliers_stat[i].Brand = orig_br
				item.Suppliers_stat[i].Articul = orig_art
				item.Suppliers_stat[i].Stock_current = stock
				item.Suppliers_stat[i].Price_current = truncatePrec(price)
				item.namesraw = append(item.namesraw, name)
				break
			}
		}
		itemsidx[brand][item.Articul] = &item
		items = append(items, &item)
	}
	if rows.Err() != nil {
		return nil, err
	}
	return items, nil
}

func getAllProducts(db *pgxpool.Pool, sups []stat_products_item_sup_stat) ([]*stat_products_item, error) {
	rows, err := db.Query(context.Background(), `SELECT p.brand, p.articul, po.origin_brand, po.origin_articul, po.sourceid, po.stock, po.price, po.name, p.category
	FROM products p JOIN products_offers po ON p.id=po.productid`)
	if err != nil {
		return nil, err
	}
	items := make([]*stat_products_item, 0)
	itemsidx := make(map[string]map[string]*stat_products_item, 0)

rowing1:
	for rows.Next() {
		var brand, articul, orig_br, orig_art, name, cat string
		var srcid, stock int
		var price float64
		if err := rows.Scan(&brand, &articul, &orig_br, &orig_art, &srcid, &stock, &price, &name, &cat); err != nil {
			rows.Close()
			return nil, err
		}

		if ff, ok := itemsidx[brand]; ok {
			if f, ok := ff[articul]; ok {
				for i := range f.Suppliers_stat {
					if f.Suppliers_stat[i].sourceid == srcid {
						if f.Suppliers_stat[i].Brand == "" {
							f.Suppliers_stat[i].Brand = orig_br
							f.Suppliers_stat[i].Articul = orig_art
							f.Suppliers_stat[i].Stock_current = stock
							f.Suppliers_stat[i].Price_current = truncatePrec(price)
							if len(name) > 0 {
								f.namesraw = append(f.namesraw, name)
							}
							break
						} else {
							break
						}
					}
				}
				continue rowing1
			}
		} else {
			itemsidx[brand] = make(map[string]*stat_products_item)
		}

		item := stat_products_item{
			Category:       cat,
			Brand:          brand,
			Articul:        articul,
			namesraw:       make([]string, 0),
			Suppliers_stat: make([]stat_products_item_sup_stat, len(sups)),
		}
		copy(item.Suppliers_stat, sups)
		if len(name) > 0 {
			item.namesraw = append(item.namesraw, name)
		}

		for i := range item.Suppliers_stat {
			if item.Suppliers_stat[i].sourceid == srcid {
				item.Suppliers_stat[i].Brand = orig_br
				item.Suppliers_stat[i].Articul = orig_art
				item.Suppliers_stat[i].Stock_current = stock
				item.Suppliers_stat[i].Price_current = truncatePrec(price)
				item.namesraw = append(item.namesraw, name)
				break
			}
		}
		itemsidx[brand][item.Articul] = &item
		items = append(items, &item)
	}
	if rows.Err() != nil {
		return nil, err
	}
	// SELECT p.brand, p.articul, po.origin_brand, po.origin_articul, po.sourceid, po.stock, po.price, po.name
	// 	FROM products p JOIN products_offers po ON p.id=po.productid
	rows, err = db.Query(context.Background(), `SELECT brand, articul, brand_raw, articul_raw, sourceid, laststock, lastprice, name, lastupdate
	FROM products_trash`)
	if err != nil {
		return nil, err
	}

	tlim := time.Now().Add(-48 * time.Hour)

rowing2:
	for rows.Next() {
		var brand, articul, orig_br, orig_art, name string
		var srcid, stock int
		var price float64
		var lastupd time.Time
		if err := rows.Scan(&brand, &articul, &orig_br, &orig_art, &srcid, &stock, &price, &lastupd); err != nil {
			rows.Close()
			return nil, err
		}

		if ff, ok := itemsidx[brand]; ok {
			if f, ok := ff[articul]; ok {
				for i := range f.Suppliers_stat {
					if f.Suppliers_stat[i].sourceid == srcid {
						if f.Suppliers_stat[i].Brand == "" {
							f.Suppliers_stat[i].Brand = orig_br
							f.Suppliers_stat[i].Articul = orig_art
							if lastupd.After(tlim) {
								f.Suppliers_stat[i].Stock_current = stock
								f.Suppliers_stat[i].Price_current = truncatePrec(price)
							}
							f.namesraw = append(f.namesraw, name)
							break
						} else {
							break
						}
					}
				}
				continue rowing2
			}
		} else {
			itemsidx[brand] = make(map[string]*stat_products_item)
		}

		item := stat_products_item{
			Category:       "",
			Brand:          brand,
			Articul:        articul,
			namesraw:       make([]string, 0),
			Suppliers_stat: make([]stat_products_item_sup_stat, len(sups)),
		}
		copy(item.Suppliers_stat, sups)

		for i := range item.Suppliers_stat {
			if item.Suppliers_stat[i].sourceid == srcid {
				item.Suppliers_stat[i].Brand = orig_br
				item.Suppliers_stat[i].Articul = orig_art
				if lastupd.After(tlim) {
					item.Suppliers_stat[i].Stock_current = stock
					item.Suppliers_stat[i].Price_current = truncatePrec(price)
				}
				item.namesraw = append(item.namesraw, name)
				break
			}
		}
		itemsidx[brand][item.Articul] = &item
		items = append(items, &item)
	}
	if rows.Err() != nil {
		return nil, err
	}

	return items, nil
}

func (item *stat_products_item) getOffersHistory(db *pgxpool.Pool) error {
	if len(item.Suppliers_stat) == 0 {
		panic("wtf")
	}
	for i := range item.Suppliers_stat {
		if item.Suppliers_stat[i].Brand == "" {
			continue
		}
		rows, err := db.Query(context.Background(), `SELECT	COALESCE(ph.price,-1), COALESCE(ph.stock,0)
		FROM products_raw p
		JOIN uploads_raw u ON u.sourceid = p.sourceid
		LEFT JOIN prices_history ph ON ph.producthash = p.hash AND ph.uploadid = u.id
		WHERE p.brand = $1 AND p.articul = $2 AND p.sourceid = $3 AND u.id>=$4 AND u.id<=$5 
		ORDER BY u.id ASC;`, item.Suppliers_stat[i].Brand, item.Suppliers_stat[i].Articul, item.Suppliers_stat[i].sourceid, item.Suppliers_stat[i].earliest_upload, item.Suppliers_stat[i].latest_upload)
		if err != nil {
			return err
		}

		var prevst int
		for rows.Next() {
			var pr float64
			var st int
			if err = rows.Scan(&pr, &st); err != nil {
				rows.Close()
				return err
			}
			if pr > 5 && st > 0 {
				item.Suppliers_stat[i].prices = append(item.Suppliers_stat[i].prices, pr)
				item.Suppliers_stat[i].stocks = append(item.Suppliers_stat[i].stocks_nozeroes, float64(st))

				if pr < item.Suppliers_stat[i].Price_min || item.Suppliers_stat[i].Price_min == 0 {
					item.Suppliers_stat[i].Price_min = pr
				}
			}
			item.Suppliers_stat[i].stocks_nozeroes = append(item.Suppliers_stat[i].stocks, float64(st))

			if d := prevst - st; d > 0 && d < diffmod_anomaly {
				item.Suppliers_stat[i].DiffStat += d
			}

			prevst = st
		}
		if rows.Err() != nil {
			return err
		}
	}

	return nil
}
