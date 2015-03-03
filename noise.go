package main

import (
	"fmt"
	"math"
	"math/rand"
	"time"
)

func generateNoiseTraders(n int) (out []trader) {
	for i := 0; i < n; i++ {
		out = append(out, newHFTTrader(newNoiseTrader(i, 6, 30)))
	}

	return
}

type noiseTrader struct {
	Id    int
	Cash  float64
	Stock int

	LastPrice float64
	MyTrades  map[float64]bool

	Latency time.Duration
	Timer   *time.Timer
}

func newNoiseTrader(id int, minLatency, maxLatency float64) *noiseTrader {
	price := (rand.Float64() * 80) + 60

	t := &noiseTrader{
		Cash:     2000 - price*10,
		Stock:    10,
		Id:       id,
		Latency:  time.Duration(math.Trunc((rand.Float64()*(maxLatency-minLatency))+minLatency)) * time.Millisecond,
		MyTrades: make(map[float64]bool),
	}
	t.Timer = time.NewTimer(t.newDuration())

	go func() {
		for {
			<-t.Timer.C

			// Make a totally random trade
			var quantity int
			if t.Stock > 1 {
				quantity = rand.Intn(t.Stock) + 1
			} else {
				quantity = rand.Intn(10) + 1
			}

			price := (rand.Float64() * 80) + 60
			sell := rand.Intn(2)
			sellBool := true
			if sell == 0 {
				sellBool = false
				if t.Cash < float64(quantity)*price {
					// can't trade right now
					t.Timer.Reset(t.newDuration())
					continue
				}

				t.Cash -= float64(quantity) * price
			} else {
				if t.Stock < quantity {
					// can't trade right now
					t.Timer.Reset(t.newDuration())
					continue
				}

				t.Stock -= quantity
			}

			fmt.Println("Noise trader", t.Id, "able to execute at", price, "for", quantity, "shares", sellBool)

			randId := rand.Float64()
			t.MyTrades[randId] = true

			go func(p float64, q int) {
				time.Sleep(t.Latency)

				listChannel <- &order{
					ID:       randId,
					Quantity: q,
					Price:    p,
					Date:     time.Now(),
					Sell:     sellBool,
				}
			}(price, quantity)

			t.Timer.Reset(t.newDuration())
		}
	}()

	return t
}

func (n *noiseTrader) newDuration() time.Duration {
	return time.Duration(math.Trunc((rand.Float64()*5)+2)) * time.Second
}

func (n *noiseTrader) newOrder(o *order) []*order {
	return nil
}

func (n *noiseTrader) filledOrder(sell, buy *order, q int, p float64) []*order {
	go func() {
		time.Sleep(n.Latency)

		if _, ok := n.MyTrades[sell.ID]; ok {
			n.Cash += float64(q) * p
		} else if _, ok := n.MyTrades[buy.ID]; ok {
			n.Stock += q
		}

		fmt.Println("Noise trader", n.Id, "has inventory", n.Cash, n.Stock)
		n.LastPrice = p
	}()
	return nil
}
