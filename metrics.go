package main

import (
	"regexp"
	"time"

	"github.com/ngrash/tgstat/backfill"
	"github.com/ngrash/tgstat/tgexport"
)

const metricsPrefix = "tg_"

const (
	tgMessagesTotal    = metricsPrefix + "messages_total"
	tgExpressionsTotal = metricsPrefix + "expressions_total"
	tgBytesTotal       = metricsPrefix + "bytes_total"
)

func analyzeChat(data *tgexport.Result, metrics *backfill.Metrics, expressions []*regexp.Regexp) error {
	for _, msg := range data.Messages {
		if msg.From == "" {
			continue
		}
		senderMetrics := metrics.With("sender", string(msg.From))

		senderMetrics.Metric(tgMessagesTotal).Inc(1, time.Time(msg.Date))
		for _, txt := range msg.TextEntities {
			senderMetrics.Metric(tgBytesTotal).Inc(uint64(len(txt.Text)), time.Time(msg.Date))
			for _, expr := range expressions {
				if expr.MatchString(txt.Text) {
					senderMetrics.Metric(tgExpressionsTotal).With("expression", expr.String()).Inc(1, time.Time(msg.Date))
				}
			}
		}
	}
	return nil
}
