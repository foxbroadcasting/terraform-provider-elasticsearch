package es

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	elastic7 "github.com/olivere/elastic/v7"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/terraform"

	"github.com/phillbaker/terraform-provider-elasticsearch/kibana"
)

func TestAccElasticsearchKibanaAlert(t *testing.T) {
	provider := Provider().(*schema.Provider)
	err := provider.Configure(&terraform.ResourceConfig{})
	if err != nil {
		t.Skipf("err: %s", err)
	}
	meta := provider.Meta()

	// We use the elasticsearch version to check compatibilty, it'll connect to
	// kibana below
	esClient, err := getClient(meta.(*ProviderConf))
	if err != nil {
		t.Skipf("err: %s", err)
	}

	var allowed bool
	switch esClient.(type) {
	case *elastic7.Client:
		allowed = true
	default:
		allowed = false
	}

	var defaultActionID string
	if allowed {
		// create and save an action for use in the tests below
		defaultActionID, err = testKibanaAlertCreateAction()
		if err != nil {
			t.Errorf("error creating action fixture: %+v", err)
		}
	}

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			if !allowed {
				t.Skip("Kibana Alerts only supported on ES >= 7.7")
			}
		},
		Providers:    testAccKibanaProviders,
		CheckDestroy: testCheckElasticsearchKibanaAlertDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccElasticsearchKibanaAlertV77(defaultActionID),
				Check: resource.ComposeTestCheckFunc(
					testCheckElasticsearchKibanaAlertExists("elasticsearch_kibana_alert.test"),
				),
			},
		},
	})
}

func TestAccElasticsearchKibanaAlert_importBasic(t *testing.T) {
	provider := Provider().(*schema.Provider)
	err := provider.Configure(&terraform.ResourceConfig{})
	if err != nil {
		t.Skipf("err: %s", err)
	}
	meta := provider.Meta()

	esClient, err := getClient(meta.(*ProviderConf))
	if err != nil {
		t.Skipf("err: %s", err)
	}

	var allowed bool
	switch esClient.(type) {
	case *elastic7.Client:
		allowed = true
	default:
		allowed = false
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			if !allowed {
				t.Skip("Kibana Alerts only supported on ES >= 7.7")
			}
		},
		Providers:    testAccKibanaProviders,
		CheckDestroy: testCheckElasticsearchKibanaAlertDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccElasticsearchKibanaAlertNoActionsV77,
			},
			{
				ResourceName:      "elasticsearch_kibana_alert.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testCheckElasticsearchKibanaAlertExists(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("Not found: %s", name)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("No kibana alert ID is set")
		}

		meta := testAccKibanaProvider.Meta()

		esClient, err := getKibanaClient(meta.(*ProviderConf))
		if err != nil {
			return err
		}

		switch client := esClient.(type) {
		case *elastic7.Client:
			_, err = kibanaGetAlert(client, rs.Primary.ID, "")
		default:
			err = errors.New("Kibana Alerts only supported on ES >= 7.7")
		}

		if err != nil {
			return err
		}

		return nil
	}
}

func testCheckElasticsearchKibanaAlertDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "elasticsearch_kibana_alert" {
			continue
		}

		meta := testAccKibanaProvider.Meta()

		esClient, err := getKibanaClient(meta.(*ProviderConf))
		if err != nil {
			return err
		}

		switch client := esClient.(type) {
		case *elastic7.Client:
			_, err = kibanaGetAlert(client, rs.Primary.ID, "")
		default:
			err = errors.New("Kibana Alerts only supported on ES >= 7.7")
		}

		if err != nil {
			return nil // should be not found error
		}

		return fmt.Errorf("kibana alert %q still exists", rs.Primary.ID)
	}

	return nil
}

func testKibanaAlertCreateAction() (string, error) {
	err := testAccKibanaProvider.Configure(&terraform.ResourceConfig{})
	if err != nil {
		return "", err
	}
	meta := testAccKibanaProvider.Meta()

	esClient, err := getKibanaClient(meta.(*ProviderConf))
	if err != nil {
		return "", err
	}

	switch client := esClient.(type) {
	case *elastic7.Client:
		res, err := client.PerformRequest(context.TODO(), elastic7.PerformRequestOptions{
			Method: "POST",
			Path:   "/api/actions/action",
			Body:   `{"name":"An index action","actionTypeId":".index","config":{"index":"foo"},"secrets":{}}`,
		})
		if err != nil {
			return "", err
		}

		action := new(kibana.AlertAction)
		if err := json.Unmarshal(res.Body, action); err != nil {
			return "", err
		}

		return action.ID, nil
	default:
		return "", errors.New("Kibana Alerts only supported on ES >= 7.7")
	}
}

func testAccElasticsearchKibanaAlertV77(actionID string) string {
	return fmt.Sprintf(`
resource "elasticsearch_kibana_alert" "test" {
  name = "terraform-alert"
  schedule {
  	interval = "1m"
  }
  conditions {
    aggregation_type = "avg"
    term_size = 6
    threshold_comparator = ">"
    time_window_size = 5
    time_window_unit = "m"
    group_by = "top"
    threshold = [1000]
    index = [".test-index"]
    time_field = "@timestamp"
    aggregation_field = "sheet.version"
    term_field = "name.keyword"
  }
  actions {
  	id = "%s"
  	action_type_id = ".index"
  	group = "threshold met"
  	params = {
  		level = "info"
  		message = "alert '{{alertName}}' is active for group '{{context.group}}':\n\n- Value: {{context.value}}\n- Conditions Met: {{context.conditions}} over {{params.timeWindowSize}}{{params.timeWindowUnit}}\n- Timestamp: {{context.date}}"
  	}
  }
}
`, actionID)
}

var testAccElasticsearchKibanaAlertNoActionsV77 = `
resource "elasticsearch_kibana_alert" "test" {
  name = "terraform-alert"
  schedule {
  	interval = "1m"
  }
  conditions {
    aggregation_type = "avg"
    term_size = 6
    threshold_comparator = ">"
    time_window_size = 5
    time_window_unit = "m"
    group_by = "top"
    threshold = [1000]
    index = [".test-index"]
    time_field = "@timestamp"
    aggregation_field = "sheet.version"
    term_field = "name.keyword"
  }
}
`

// var testAccElasticsearchKibanaAlertV711 = `
// resource "elasticsearch_kibana_alert" "test" {
//   name = "terraform-alert"
//   notify_when = "onActionGroupChange"
//   schedule {
//   	interval = "1m"
//   }
//   conditions {
//     aggregation_type = "avg"
//     term_size = 6
//     threshold_comparator = ">"
//     time_window_size = 5
//     time_window_unit = "m"
//     group_by = "top"
//     threshold = [1000]
//     index = [".test-index"]
//     time_field = "@timestamp"
//     aggregation_field = "sheet.version"
//     term_field = "name.keyword"
//   }
// }
// `
