package mimir

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func getSetEnv(key, fallback string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		value = fallback
		os.Setenv(key, fallback)
	}
	return value
}

func testAccCheckMimirRuleGroupExists(n string, name string, client *apiClient) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			keys := make([]string, 0, len(s.RootModule().Resources))
			for k := range s.RootModule().Resources {
				keys = append(keys, k)
			}
			return fmt.Errorf("mimir object not found in terraform state: %s. Found: %s", n, strings.Join(keys, ", "))
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("mimir object name %s not set in terraform", name)
		}

		orgID := rs.Primary.Attributes["org_id"]
		name := rs.Primary.Attributes["name"]
		namespace := rs.Primary.Attributes["namespace"]

		/* Make a throw-away API object to read from the API */
		headers := make(map[string]string)
		if orgID != "" {
			headers["X-Scope-OrgID"] = orgID
		}
		path := fmt.Sprintf("/config/v1/rules/%s/%s", namespace, name)
		_, err := client.sendRequest("ruler", "GET", path, "", headers)
		if err != nil {
			return err
		}

		return nil
	}
}

func testAccCheckMimirRuleGroupDestroy(s *terraform.State) error {
	// retrieve the connection established in Provider configuration
	client := testAccProvider.Meta().(*apiClient)

	// loop through the resources in state, verifying each widget
	// is destroyed
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "mimir_rule_group_recording" {
			continue
		}

		orgID := rs.Primary.Attributes["org_id"]
		name := rs.Primary.Attributes["name"]
		namespace := rs.Primary.Attributes["namespace"]

		headers := make(map[string]string)
		if orgID != "" {
			headers["X-Scope-OrgID"] = orgID
		}
		path := fmt.Sprintf("/config/v1/rules/%s/%s", namespace, name)
		_, err := client.sendRequest("ruler", "GET", path, "", headers)

		// If the error is equivalent to 404 not found, the widget is destroyed.
		// Otherwise return the error
		if !strings.Contains(err.Error(), "group does not exist") {
			return err
		}
	}

	return nil
}

func setupClient() *apiClientOpt {
	headers := make(map[string]string)
	headers["X-Scope-OrgID"] = mimirOrgID

	opt := &apiClientOpt{
		uri:             mimirURI,
		rulerURI:        mimirRulerURI,
		alertmanagerURI: mimirAlertmanagerURI,
		insecure:        false,
		username:        "",
		password:        "",
		proxyURL:        "",
		token:           "",
		cert:            "",
		key:             "",
		ca:              "",
		headers:         headers,
		timeout:         2,
		debug:           true,
	}
	return opt
}

// TestValidatePromQLExpr_ExperimentalFunctions tests the validatePromQLExpr function
// with experimental PromQL functions enabled and disabled
func TestValidatePromQLExpr_ExperimentalFunctions(t *testing.T) {
	tests := []struct {
		name                              string
		expr                              string
		enableExperimentalPromQLFunctions bool
		expectError                       bool
		errorContains                     string
	}{
		{
			name:                              "standard function without experimental flag",
			expr:                              "rate(http_requests_total[5m])",
			enableExperimentalPromQLFunctions: false,
			expectError:                       false,
		},
		{
			name:                              "standard function with experimental flag",
			expr:                              "rate(http_requests_total[5m])",
			enableExperimentalPromQLFunctions: true,
			expectError:                       false,
		},
		{
			name:                              "experimental function without experimental flag",
			expr:                              "double_exponential_smoothing(http_requests_total[5m], 0.1, 0.1)",
			enableExperimentalPromQLFunctions: false,
			expectError:                       true,
			errorContains:                     "is not enabled",
		},
		{
			name:                              "experimental function with experimental flag",
			expr:                              "double_exponential_smoothing(http_requests_total[5m], 0.1, 0.1)",
			enableExperimentalPromQLFunctions: true,
			expectError:                       false,
		},
		{
			name:                              "invalid PromQL expression without experimental flag",
			expr:                              "rate(invalid_syntax",
			enableExperimentalPromQLFunctions: false,
			expectError:                       true,
			errorContains:                     "Invalid PromQL expression",
		},
		{
			name:                              "invalid PromQL expression with experimental flag",
			expr:                              "rate(invalid_syntax",
			enableExperimentalPromQLFunctions: true,
			expectError:                       true,
			errorContains:                     "Invalid PromQL expression",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save the original value and restore it at the end of the test
			originalValue := enableExperimentalPromQLFunctions
			defer func() {
				enableExperimentalPromQLFunctions = originalValue
			}()

			// Set the experimental functions flag for this test
			enableExperimentalPromQLFunctions = tt.enableExperimentalPromQLFunctions

			// Call the validation function
			_, errors := validatePromQLExpr(tt.expr, "test_field")

			// Check if we got the expected result
			if tt.expectError {
				if len(errors) == 0 {
					t.Errorf("Expected validation error for expression %q with experimental functions %v, but got none",
						tt.expr, tt.enableExperimentalPromQLFunctions)
				} else if tt.errorContains != "" {
					found := false
					for _, err := range errors {
						if strings.Contains(err.Error(), tt.errorContains) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Expected error containing %q, but got errors: %v", tt.errorContains, errors)
					}
				}
			} else {
				if len(errors) > 0 {
					t.Errorf("Expected no validation error for expression %q with experimental functions %v, but got: %v",
						tt.expr, tt.enableExperimentalPromQLFunctions, errors)
				}
			}
		})
	}
}

// TestFormatPromQLExpr_ExperimentalFunctions tests the formatPromQLExpr function
// with experimental PromQL functions enabled and disabled
func TestFormatPromQLExpr_ExperimentalFunctions(t *testing.T) {
	tests := []struct {
		name                              string
		expr                              string
		enablePromQLExprFormat            bool
		enableExperimentalPromQLFunctions bool
		expectedFormatted                 bool
	}{
		{
			name:                              "standard function with formatting disabled",
			expr:                              "rate(http_requests_total[5m])",
			enablePromQLExprFormat:            false,
			enableExperimentalPromQLFunctions: false,
			expectedFormatted:                 false,
		},
		{
			name:                              "standard function with formatting enabled, experimental disabled",
			expr:                              "rate(http_requests_total[5m])",
			enablePromQLExprFormat:            true,
			enableExperimentalPromQLFunctions: false,
			expectedFormatted:                 true,
		},
		{
			name:                              "experimental function with formatting enabled, experimental enabled",
			expr:                              "double_exponential_smoothing(http_requests_total, 0.1, 0.1)",
			enablePromQLExprFormat:            true,
			enableExperimentalPromQLFunctions: true,
			expectedFormatted:                 true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save the original values and restore them at the end of the test
			originalFormatValue := enablePromQLExprFormat
			originalExperimentalValue := enableExperimentalPromQLFunctions
			defer func() {
				enablePromQLExprFormat = originalFormatValue
				enableExperimentalPromQLFunctions = originalExperimentalValue
			}()

			// Set the flags for this test
			enablePromQLExprFormat = tt.enablePromQLExprFormat
			enableExperimentalPromQLFunctions = tt.enableExperimentalPromQLFunctions

			// Call the formatting function
			result := formatPromQLExpr(tt.expr)

			// Check if formatting was applied
			if tt.expectedFormatted {
				// If formatting is enabled, the result should be different from the input
				// (unless the input was already perfectly formatted)
				if !tt.enablePromQLExprFormat {
					t.Errorf("Expected formatting to be disabled, but enablePromQLExprFormat is %v", tt.enablePromQLExprFormat)
				}
				// The result should be a valid string (not empty)
				if result == "" {
					t.Errorf("Expected formatted result, but got empty string")
				}
			} else if result != tt.expr {
				// If formatting is disabled, the result should be the same as the input
				t.Errorf("Expected unformatted result %q, but got %q", tt.expr, result)
			}
		})
	}
}
