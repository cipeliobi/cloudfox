package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/subscriptions"
	"github.com/BishopFox/cloudfox/globals"
	"github.com/BishopFox/cloudfox/utils"
	"github.com/aws/smithy-go/ptr"
	"github.com/fatih/color"
)

type scopeElement struct {
	// Use for user selection in interactive mode.
	menuIndex int
	// True will cause CloudFox to enumerate the resource group.
	includeInCloudFoxExecution bool
	ResourceGroup              resources.Group
	Sub                        subscriptions.Subscription
	Tenant                     subscriptions.TenantIDDescription
}

// userInput = nil will prompt interactive menu for RG selection.
// The userInput argument is used to toggle the interactive menu (useful for unit tests).
// mode = full (prints entire table), tenant (prints only tenants table)
func ScopeSelection(userInput *string, mode string) []scopeElement {
	fmt.Printf("[%s] Fetching available resource groups from Az CLI sessions...\n", color.CyanString(globals.AZ_INTERACTIVE_MENU_MODULE_NAME))
	var results []scopeElement

	availableScope := getAvailableScope()
	switch mode {
	default:
		printAvailableScopeFull(availableScope)
	}

	if userInput == nil {
		var input string
		fmt.Printf("[%s] Please make a selection (e.g. '1' or '1,2,3').\n", color.CyanString(globals.AZ_INTERACTIVE_MENU_MODULE_NAME))
		fmt.Printf("[%s]> ", color.CyanString(globals.AZ_INTERACTIVE_MENU_MODULE_NAME))
		fmt.Scanln(&input)
		userInput = ptr.String(input)
	}

	for _, scopeItem := range availableScope {
		for _, userSelection := range strings.Split(ptr.ToString(userInput), ",") {
			userInputInt, err := strconv.Atoi(userSelection)
			if err != nil {
				log.Fatalln("Error: Invalid resource group selection.")
			}
			if userInputInt == scopeItem.menuIndex {
				results = append(
					results,
					scopeElement{
						menuIndex:                  scopeItem.menuIndex,
						includeInCloudFoxExecution: true,
						Sub:                        scopeItem.Sub,
						ResourceGroup:              scopeItem.ResourceGroup})
			}
		}
	}
	return results
}

func printAvailableScopeFull(availableScope []scopeElement) {
	var tableBody [][]string

	for _, scopeItem := range availableScope {
		tableBody = append(
			tableBody,
			[]string{
				strconv.Itoa(scopeItem.menuIndex),
				ptr.ToString(scopeItem.ResourceGroup.Name),
				ptr.ToString(scopeItem.Sub.DisplayName),
				ptr.ToString(scopeItem.Tenant.DisplayName),
				ptr.ToString(scopeItem.Tenant.DefaultDomain),
			})
	}
	sort.Slice(
		tableBody,
		func(i int, j int) bool {
			return tableBody[i][0] < tableBody[j][0]
		},
	)
	utils.PrintTableToScreen(
		[]string{
			"#",
			"Resource Group",
			"Subscription",
			"Tenant Name",
			"Domain",
		},
		tableBody)
}

func GetSubscriptionForResourceGroup(resourceGroupName string) subscriptions.Subscription {
	availableScope := getAvailableScope()
	for _, s := range availableScope {
		if ptr.ToString(s.ResourceGroup.Name) == resourceGroupName {
			return s.Sub
		}
	}
	return subscriptions.Subscription{}
}

func GetSubscriptionForResourceGroup_LEGACY(resourceGroupName string) subscriptions.Subscription {
	subs := GetSubscriptions()
	for _, sub := range subs {
		rgs := GetResourceGroups(ptr.ToString(sub.SubscriptionID))
		for _, rg := range rgs {
			if ptr.ToString(rg.Name) == resourceGroupName {
				return sub
			}
		}
	}
	return subscriptions.Subscription{}
}

func getAvailableScope() []scopeElement {
	var index int
	var results []scopeElement
	tenants := GetTenants()
	subscriptions := GetSubscriptions()

	for _, t := range tenants {
		for _, s := range subscriptions {
			if ptr.ToString(t.TenantID) == ptr.ToString(s.TenantID) {
				for _, rg := range GetResourceGroups(ptr.ToString(s.SubscriptionID)) {
					index++
					results = append(results, scopeElement{
						menuIndex:                  index,
						includeInCloudFoxExecution: false,
						ResourceGroup:              rg,
						Sub:                        s,
						Tenant:                     t,
					})
				}
			}
		}
	}
	return results
}

var GetTenants = getTenants

func getTenants() []subscriptions.TenantIDDescription {
	tenantsClient := utils.GetTenantsClient()
	var results []subscriptions.TenantIDDescription
	for page, err := tenantsClient.List(context.TODO()); page.NotDone(); err = page.Next() {
		if err != nil {
			log.Fatal("could not get tenants for active session")
		}
		results = append(results, page.Values()...)
	}
	return results
}

var GetSubscriptions = getSubscriptions

func getSubscriptions() []subscriptions.Subscription {
	var results []subscriptions.Subscription
	subsClient := utils.GetSubscriptionsClient()
	for page, err := subsClient.List(context.TODO()); page.NotDone(); err = page.Next() {
		if err != nil {
			log.Fatal("could not get subscriptions for active session")
		}
		results = append(results, page.Values()...)
	}
	return results
}

var GetResourceGroups = getResourceGroups

func getResourceGroups(subscriptionID string) []resources.Group {
	var results []resources.Group
	rgClient := utils.GetResourceGroupsClient(subscriptionID)

	for page, err := rgClient.List(context.TODO(), "", nil); page.NotDone(); err = page.Next() {
		if err != nil {
			log.Fatalf("error reading resource groups for subscription %s", subscriptionID)
		}
		results = append(results, page.Values()...)
	}
	return results
}

/************* MOCKED FUNCTIONS BELOW (USE IT FOR UNIT TESTING) *************/

func MockedGetResourceGroups(subscriptionID string) []resources.Group {
	var results []resources.Group
	for _, tenant := range loadTestFile(globals.RESOURCES_TEST_FILE).Tenants {
		for _, sub := range tenant.Subscriptions {
			if ptr.ToString(sub.SubscriptionId) == subscriptionID {
				for _, rg := range sub.ResourceGroups {
					results = append(results, resources.Group{
						ID:   rg.ID,
						Name: rg.Name,
					})
				}
			}
		}
	}
	return results
}

func MockedGetSubscriptions() []subscriptions.Subscription {
	var results []subscriptions.Subscription
	for _, tenant := range loadTestFile(globals.RESOURCES_TEST_FILE).Tenants {
		for _, sub := range tenant.Subscriptions {
			results = append(results, subscriptions.Subscription{
				TenantID:       tenant.TenantID,
				SubscriptionID: sub.SubscriptionId,
				DisplayName:    sub.DisplayName,
			})
		}
	}
	return results
}

func MockedGetTenants() []subscriptions.TenantIDDescription {
	var results []subscriptions.TenantIDDescription
	for _, tenant := range loadTestFile(globals.RESOURCES_TEST_FILE).Tenants {
		results = append(results, subscriptions.TenantIDDescription{
			TenantID:      tenant.TenantID,
			DisplayName:   tenant.DisplayName,
			DefaultDomain: tenant.DefaultDomain,
		})
	}
	return results
}

func loadTestFile(fileName string) ResourcesTestFile {
	file, err := os.ReadFile(fileName)
	if err != nil {
		log.Fatalf("could not read file %s", globals.RESOURCES_TEST_FILE)
	}
	var testFile ResourcesTestFile
	err = json.Unmarshal(file, &testFile)
	if err != nil {
		log.Fatalf("could not unmarshall file %s", globals.RESOURCES_TEST_FILE)
	}
	return testFile
}

type ResourcesTestFile struct {
	Tenants []struct {
		DisplayName   *string `json:"displayName"`
		TenantID      *string `json:"tenantId"`
		DefaultDomain *string `json:"defaultDomain,omitempty"`
		Subscriptions []struct {
			DisplayName    *string `json:"displayName"`
			SubscriptionId *string `json:"subscriptionId"`
			ResourceGroups []struct {
				Name *string `json:"Name"`
				ID   *string `json:"id"`
			} `json:"ResourceGroups"`
		} `json:"Subscriptions"`
	} `json:"Tenants"`
}
