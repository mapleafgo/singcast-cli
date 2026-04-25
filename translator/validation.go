package translator

import "fmt"

func validate(t *translation) error {
	tagSet := map[string]bool{"DIRECT": true}

	// Collect all outbound tags and check for duplicates
	for _, ob := range t.config.Outbounds {
		tag, _ := ob["tag"].(string)
		if tag == "" {
			continue
		}
		if tagSet[tag] {
			// Allow the assemble-injected DIRECT to coexist
			if tag == "DIRECT" {
				continue
			}
			return fmt.Errorf("duplicate outbound tag: %s", tag)
		}
		tagSet[tag] = true
	}

	// Verify rule outbound references
	for _, rule := range t.config.Route.Rules {
		outbound, _ := rule["outbound"].(string)
		if outbound != "" && !tagSet[outbound] {
			t.warn("rule references non-existent outbound: " + outbound)
		}
	}

	// Verify route.final
	if t.config.Route.Final != "" && !tagSet[t.config.Route.Final] {
		return fmt.Errorf("route.final references non-existent outbound: %s", t.config.Route.Final)
	}

	return nil
}
