package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func widgetTestwrap(test func(*Task)) {
	rq := http.Request{}
	dbTest(&rq, test)
	if _, err := db.Exec(`DELETE FROM widgets`); err != nil {
		panic(err)
	}
}

func TestWidgets_post_widget(t *testing.T) {
	widgetTestwrap(func(task *Task) {
		w := map[string]interface{}{
			"widget": map[string]interface{}{
				"created": nil,
				"type":    "test",
				"config": map[string]interface{}{
					"type": "LineGraph",
					"items": []interface{}{
						map[string]interface{}{
							"type":        "line",
							"title":       "Home Page",
							"color":       "#f00",
							"dataType":    "statsd",
							"dataMetric":  "pageviews.home",
							"dataChannel": "counter",
						},
					},
				},
				"dashboard": "80ef1a01-3403-4d7e-8d74-70cd951dd2e9",
			},
		}

		raw, _ := json.Marshal(w)

		req, _ := http.NewRequest("POST", "/", strings.NewReader(string(raw[:])))
		task.Rq = req

		WidgetRouter.Serve(task)
		if task.Rw.StatusCode != 200 {
			t.Errorf("Response Code should be 200")
		}
	})
}
