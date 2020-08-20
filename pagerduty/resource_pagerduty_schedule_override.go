package pagerduty

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/mdevens/go-pagerduty/pagerduty"
	"github.com/relvacode/iso8601"
)

func resourcePagerDutyScheduleOverride() *schema.Resource {
	return &schema.Resource{
		Create: resourcePagerDutyScheduleOverrideCreate,
		Read:   resourcePagerDutyScheduleOverrideRead,
		Delete: resourcePagerDutyScheduleOverrideDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},
		Schema: map[string]*schema.Schema{
			"user": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"start": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"end": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"schedule": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func buildScheduleOverrideStruct(d *schema.ResourceData) (*pagerduty.Override, error) {
	override := &pagerduty.Override{
		User: &pagerduty.UserReference{
			ID:   d.Get("user").(string),
			Type: "user",
		},
		Start: d.Get("start").(string),
		End:   d.Get("end").(string),
	}
	return override, nil
}

func resourcePagerDutyScheduleOverrideCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*pagerduty.Client)

	override, err := buildScheduleOverrideStruct(d)
	if err != nil {
		return err
	}

	log.Printf("[INFO] Creating override for PagerDuty schedule: %s", d.Get("schedule").(string))

	newOverride, _, err := client.Schedules.CreateOverride(d.Get("schedule").(string), override)
	if err != nil {
		log.Printf("[ERROR] Error creating PagerDuty schedule override: %s, checking timeframe.", d.Id())
		configStartTime, err := iso8601.ParseString(d.Get("start").(string))
		configEndTime, err := iso8601.ParseString(d.Get("end").(string))

		now := time.Now()

		if configEndTime.Before(now) || configStartTime.Before(now) {
			log.Printf("[INFO] You tried to create an old Override. Ignore API Response and label as created")
			d.SetId(fmt.Sprintf("IGNORED_%d", rand.Int()))
			return resourcePagerDutyScheduleOverrideRead(d, meta)
		}

		return err
	}

	d.SetId(newOverride.ID)

	return resourcePagerDutyScheduleOverrideRead(d, meta)
}

func resourcePagerDutyScheduleOverrideRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*pagerduty.Client)

	log.Printf("[INFO] Reading PagerDuty schedule override: %s", d.Id())

	listOverridesOptions := pagerduty.ListOverridesOptions{
		Since: d.Get("start").(string),
		Until: d.Get("end").(string),
	}

	overrides, _, err := client.Schedules.ListOverrides(d.Get("schedule").(string), &listOverridesOptions)
	if err != nil {
		return err
	}

	var matchingOverrides []*pagerduty.Override
	for _, o := range overrides.Overrides {
		if o.ID == d.Id() {
			matchingOverrides = append(matchingOverrides, o)
		}
	}
	if len(matchingOverrides) != 1 {
		if strings.HasPrefix(d.Id(), "IGNORED") {
			log.Printf("[INFO] API couldn't find the override, but it was labeled as ignored, so, will ignore")
			return nil
		}

		err := errors.New(fmt.Sprintf("Could not find override: %s", d.Id()))
		return handleNotFoundError(err, d)
	}

	d.Set("user", matchingOverrides[0].User.ID)

	// different layouts - making both available to the users
	configStartTime, err := iso8601.ParseString(d.Get("start").(string))
	configEndTime, err := iso8601.ParseString(d.Get("end").(string))
	apiStartTime, _ := iso8601.ParseString(matchingOverrides[0].Start)
	apiEndTime, _ := iso8601.ParseString(matchingOverrides[0].End)
	now := time.Now()

	// checking old overrides
	if configEndTime.Before(now) || configStartTime.Before(now) {
		log.Printf("[INFO] You tried to read an old Override, return as nothing happened and assume state is correct")
		return nil
	}

	if !configStartTime.Equal(apiStartTime) {
		d.Set("start", matchingOverrides[0].Start)
	}

	if !configEndTime.Equal(apiEndTime) {
		d.Set("end", matchingOverrides[0].End)
	}

	return nil
}

func resourcePagerDutyScheduleOverrideDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*pagerduty.Client)

	log.Printf("[INFO] Deleting PagerDuty schedule override: %s", d.Id())

	_, err := client.Schedules.DeleteOverride(d.Get("schedule").(string), d.Id())
	if err != nil {
		log.Printf("[ERROR] Error deleting PagerDuty schedule override: %s, going alternate route.", d.Id())
		end, _ := time.Parse(time.RFC3339, d.Get("end").(string))

		now := time.Now()

		// Check if it is an override in the past, then just let it delete from the state
		if end.Before(now) {
			log.Printf("[INFO] Old Override. Ignore API Response and label as deleted")
			d.SetId("")
			return nil
		}

		return err
	}

	d.SetId("")
	return nil
}
