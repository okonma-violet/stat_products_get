package main

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"time"

	"github.com/big-larry/suckhttp"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/okonma-violet/services/logs/logger"
	"github.com/okonma-violet/services/universalservice_nonepoll"
)

// read this from configfile
type config struct {
}

// your shit here
type service struct {
	postgresdb *pgxpool.Pool
}

const thisServiceName universalservice_nonepoll.ServiceName = "stat_products_get"

const postgres_connstr = "postgres://ozon:q13471347@127.0.0.1:5432/ozondb"

const diffmod_anomaly = 40

// const diffmod_anomaly_neg = -40

type stat_products_get_req struct {
	Date_from int64  `json:"date_from"`
	Date_to   int64  `json:"date_to"`
	Category  string `json:"category"`
	Suppliers []int  `json:"suppliers"`
}

type stat_products_get_resp struct {
	Items     []*stat_products_item    `json:"items"`
	Date_from int64                    `json:"date_from"`
	Date_to   int64                    `json:"date_to"`
	Category  string                   `json:"category"`
	Suppliers []stat_products_item_sup `json:"suppliers"`
}

type stat_products_item_sup struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type stat_products_item struct {
	// prices          []float64
	// stocks          []float64
	// stocks_nozeroes []float64

	Brand    string `json:"brand"`
	Articul  string `json:"articul"`
	Category string `json:"category"`

	namesraw []string
	Names    string `json:"names"`
	// DiffStat                 int                      `json:"diffstat"`
	// Stock_dispersion_overall float64                  `json:"stock_dispersion_overall"`
	// Stock_dispersion_avg     float64                  `json:"stock_dispersion_avg"`
	// Stock_dispersion_sum     float64                  `json:"stock_dispersion_sum"`
	// Stock_avg                float64                  `json:"stock_avg"`
	// Stock_avg_nozeroes       float64                  `json:"stock_avg_nozeroes"`
	// Stock_avg_nozeroes_sum   float64                  `json:"stock_avg_nozeroes_sum"`
	// Price_avg                float64                  `json:"price_avg"`
	// Price_min                float64                  `json:"price_min"`
	Suppliers_stat []stat_products_item_sup_stat `json:"suppliers_stat"`
}

type stat_products_item_sup_stat struct {
	supplierid           int
	sourceid             int
	earliest_upload      int
	latest_upload        int
	earliest_upload_time time.Time
	latest_upload_time   time.Time
	not_actual           bool

	prices          []float64
	stocks          []float64
	stocks_nozeroes []float64
	stocks_noanom   []float64

	Brand   string `json:"brand"`
	Articul string `json:"articul"`

	Supplier_name string `json:"sup_name"`
	Uploads       int    `json:"uploads"`

	DiffStat           int     `json:"diffstat"`
	Stock_dispersion   float64 `json:"stock_dispersion"`
	Stock_avg          float64 `json:"stock_avg"`
	Stock_avg_nozeroes float64 `json:"stock_avg_nozeroes"`
	Price_avg          float64 `json:"price_avg"`
	Price_min          float64 `json:"price_min"`
	Stock_current      int     `json:"stock_current"`
	Price_current      float64 `json:"price_current"`
}

func (c *config) InitFlags() {
}

func (c *config) PrepareHandling(ctx context.Context, pubs_getter universalservice_nonepoll.Publishers_getter) (universalservice_nonepoll.BaseHandleFunc, universalservice_nonepoll.Closer, error) {
	s := &service{}
	var err error
	if s.postgresdb, err = openPostgresDBRepository(postgres_connstr); err != nil {
		return nil, nil, errors.New("openPostgresDBRepository err: " + err.Error())
	}

	return universalservice_nonepoll.CreateHTTPHandleFunc(s), s, nil
}

func (s *service) HandleHTTP(r *suckhttp.Request, l logger.Logger) (*suckhttp.Response, error) {
	defer runtime.GC()
	// if !strings.Contains(r.GetHeader(suckhttp.Content_Type), "application/json") {
	// 	l.Debug("req", "wrong content-type")
	// 	return suckhttp.NewResponse(400, "Bad request"), nil
	// }

	reqdata := stat_products_get_req{}
	var err error
	if err = json.Unmarshal(r.Body, &reqdata); err != nil {
		l.Error("unmarshal", err)
		return suckhttp.NewResponse(400, "Bad request"), nil
	}

	var from, to time.Time
	if reqdata.Date_from != 0 {
		from = time.Unix(reqdata.Date_from, 0)
	} else {
		l.Error("req", errors.New("zero \"date_from\""))
		return suckhttp.NewResponse(400, "Bad request"), nil
	}

	if reqdata.Date_to != 0 {
		to = time.Unix(reqdata.Date_to, 0)
		// to = to.Add(time.Microsecond * 86399999999) // 23:59:59.999999
	} else {
		l.Error("req", errors.New("zero \"date_to\""))
		return suckhttp.NewResponse(400, "Bad request"), nil
	}

	if len(reqdata.Suppliers) == 0 {
		l.Error("req", errors.New("no \"suppliers\""))
		return suckhttp.NewResponse(400, "Bad request"), nil
	}

	resp := stat_products_get_resp{
		Items:     make([]*stat_products_item, 0),
		Date_from: reqdata.Date_from,
		Date_to:   reqdata.Date_to,
	}

	if reqdata.Category == "" {
		resp.Category = "all cats"
	} else {
		resp.Category = reqdata.Category
	}

	srcsup_list, sl, err := getSuppliersSourced(s.postgresdb, reqdata.Suppliers)
	if err != nil {
		l.Error("getSuppliersSourced", err)
		return nil, nil
	}

	resp.Suppliers = sl

	for i := range srcsup_list {
		if err = srcsup_list[i].getUploadsStat(s.postgresdb, from, to); err != nil {
			l.Error("getUploadsStat", err)
			return nil, nil
		}
		srcsup_list[i].checkActual()
	}

	if reqdata.Category != "" {
		if resp.Items, err = getAllProductsCated(s.postgresdb, reqdata.Category, srcsup_list); err != nil {
			l.Error("getAllProductsCated", err)
			return nil, nil
		}
	} else {
		if resp.Items, err = getAllProducts(s.postgresdb, srcsup_list); err != nil {
			l.Error("getAllProducts", err)
			return nil, nil
		}
	}

	for _, item := range resp.Items {
		if err = item.getOffersHistory(s.postgresdb); err != nil {
			l.Error("getOffersHistory", err)
			return nil, nil
		}
		item.calcStats()
		item.Names = strings.Join(item.namesraw, "\n")
	}

	var body []byte

	body, err = json.Marshal(resp)
	if err != nil {
		l.Error("Marshal", err)
		return nil, nil
	}
	respCT := "application/json"

	return suckhttp.NewResponse(200, "OK").SetBody(body).SetHeader(suckhttp.Content_Type, respCT), nil
}

func (s *service) Close(l logger.Logger) error {
	if s.postgresdb != nil {
		s.postgresdb.Close()
	}
	return nil
}

func main() {
	universalservice_nonepoll.InitNewService(thisServiceName, &config{}, 1)
}
