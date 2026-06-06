package capability_test

import (
	"fmt"
	"log"

	"github.com/Mawar2/Kaimi/internal/capability"
)

// ExampleLoadProfile demonstrates loading the BlueMeta capability profile
// and accessing its data.
func ExampleLoadProfile() {
	// Load the BlueMeta capability profile
	profile, err := capability.LoadProfile("../../config/bluemeta_profile.yaml")
	if err != nil {
		log.Fatalf("Failed to load profile: %v", err)
	}

	// Display company info
	fmt.Printf("Company: %s\n", profile.Company)
	fmt.Printf("UEI: %s\n", profile.UEI)
	fmt.Printf("CAGE: %s\n", profile.CAGE)

	// Display set-aside eligibility
	fmt.Printf("\nSmall Business: %v\n", profile.SetAside.SmallBusiness)
	fmt.Printf("SDB: %v\n", profile.SetAside.SDB)
	fmt.Printf("Minority-Owned: %v\n", profile.SetAside.MinorityOwned)

	// Display NAICS code count by tier
	primaryCodes := profile.GetNAICSByTier(capability.TierPrimary)
	secondaryCodes := profile.GetNAICSByTier(capability.TierSecondary)
	tertiaryCodes := profile.GetNAICSByTier(capability.TierTertiary)

	fmt.Printf("\nNAICS Codes:\n")
	fmt.Printf("  Primary: %d codes\n", len(primaryCodes))
	fmt.Printf("  Secondary: %d codes\n", len(secondaryCodes))
	fmt.Printf("  Tertiary: %d codes\n", len(tertiaryCodes))

	// Display clearance
	fmt.Printf("\nClearance: %s\n", profile.Clearance)

	// Display competency count
	fmt.Printf("Competencies: %d total\n", len(profile.Competencies))

	// Display past performance count
	fmt.Printf("Past Performance: %d projects\n", len(profile.PastPerformance))

	// Check eligibility for different set-asides
	fmt.Printf("\nEligibility Checks:\n")
	fmt.Printf("  Small Business Set-Aside: %v\n", profile.IsEligibleForSetAside("small-business"))
	fmt.Printf("  8(a) Set-Aside: %v\n", profile.IsEligibleForSetAside("8a"))
	fmt.Printf("  Full-and-Open: %v\n", profile.IsEligibleForSetAside("full-and-open"))

	// Output:
	// Company: BlueMeta Technologies
	// UEI: XVUEA59LY579
	// CAGE: 9RY40
	//
	// Small Business: true
	// SDB: true
	// Minority-Owned: true
	//
	// NAICS Codes:
	//   Primary: 3 codes
	//   Secondary: 3 codes
	//   Tertiary: 5 codes
	//
	// Clearance: Public Trust
	// Competencies: 16 total
	// Past Performance: 9 projects
	//
	// Eligibility Checks:
	//   Small Business Set-Aside: true
	//   8(a) Set-Aside: false
	//   Full-and-Open: true
}
