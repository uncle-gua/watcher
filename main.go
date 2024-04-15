package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/robfig/cron/v3"
	"github.com/uncle-gua/gobinance/futures"
	"github.com/uncle-gua/log"
)

type Account struct {
	Name      string  `json:"name"`
	ApiKey    string  `json:"apiKey"`
	ApiSecret string  `json:"apiSecret"`
	Profit    float64 `json:"profit"`
	Leverage  float64 `json:"leverage"`
	Spec      string  `json:"spec"`
}

func main() {
	body, err := os.ReadFile("./config.json")
	if err != nil {
		log.Fatal(err)
	}

	var accounts []Account
	if err := json.Unmarshal(body, &accounts); err != nil {
		log.Fatal(err)
	}

	for _, account := range accounts {
		c := cron.New(cron.WithSeconds())
		c.AddFunc(account.Spec, func() {
			if leverage, err := run(account); err != nil {
				log.Error(err)
			} else {
				if leverage < account.Leverage {
					c.Stop()
				}
			}
		})
		c.Start()
	}

	chExit := make(chan struct{})
	go func(ch chan struct{}) {
		chSignal := make(chan os.Signal, 1)
		signal.Notify(chSignal, syscall.SIGINT, syscall.SIGTERM)

		<-chSignal
		close(ch)
	}(chExit)

	<-chExit
}

func run(a Account) (float64, error) {
	leverage := 0.0
	client := futures.NewClient(a.ApiKey, a.ApiSecret)
	account, err := client.NewGetAccountService().Do(context.Background())
	if err != nil {
		return leverage, err
	}

	total := 0.0
	for _, position := range account.Positions {
		if position.PositionAmt > 0 {
			value := position.PositionAmt * position.EntryPrice
			roe := position.UnrealizedProfit / value * 100
			if roe > a.Profit {
				if _, err := client.NewCreateOrderService().
					Symbol(position.Symbol).
					Type(futures.OrderTypeMarket).
					Side(futures.SideTypeSell).
					PositionSide(futures.PositionSideTypeLong).
					Quantity(fmt.Sprintf("%f", position.PositionAmt)).
					Do(context.Background()); err != nil {
					return leverage, err
				}
				log.Infof("account: %s, symbol: %s, pnl: %.2f, roe: %.2f", a.Name, position.Symbol, position.UnrealizedProfit, roe)
			} else {
				total += value
			}
		}
	}

	leverage = total / account.TotalWalletBalance
	log.Infof("account: %s, leverage: %.2f", a.Name, leverage)
	return leverage, err
}
