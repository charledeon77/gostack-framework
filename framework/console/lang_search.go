package console

import (
	"fmt"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
	"strings"
)

// LangSearchCommand enables command-line search for standard translation locale codes,
// native names, and flag emojis.
type LangSearchCommand struct{}

func (c *LangSearchCommand) Name() string {
	return "lang:search"
}

func (c *LangSearchCommand) Description() string {
	return "Search for standard language codes, native names, and flag emojis (Usage: lang:search <query>)"
}

func (c *LangSearchCommand) Execute(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("lang:search failed: search query argument is missing. Example: gost lang:search spanish")
	}

	query := strings.ToLower(args[0])

	// Base language mappings to default region codes for flag resolution
	baseRegions := map[string]string{
		"en": "US", "es": "ES", "hi": "IN", "fr": "FR", "de": "DE",
		"it": "IT", "ja": "JP", "ru": "RU", "zh": "CN", "pt": "BR",
		"ar": "SA", "bn": "BD", "ko": "KR", "pa": "IN", "ta": "IN",
		"te": "IN", "tr": "TR", "vi": "VN", "mr": "IN", "jv": "ID",
		"pl": "PL", "nl": "NL", "sv": "SE", "no": "NO", "da": "DK",
		"fi": "FI", "th": "TH", "el": "GR", "he": "IL", "fa": "IR",
		"uk": "UA", "ro": "RO", "hu": "HU", "cs": "CZ", "sk": "SK",
		"id": "ID", "ms": "MY",
	}

	// List of common locales/regions for lookup matching
	locales := []string{
		"en", "en-US", "en-GB", "en-CA", "en-AU", "en-NG",
		"es", "es-ES", "es-MX", "es-AR", "es-CO",
		"hi", "hi-IN",
		"fr", "fr-FR", "fr-CA",
		"de", "de-DE",
		"it", "it-IT",
		"ja", "ja-JP",
		"ru", "ru-RU",
		"zh", "zh-CN", "zh-TW",
		"pt", "pt-PT", "pt-BR",
		"ar", "ar-SA", "ar-EG",
		"bn", "bn-BD", "bn-IN",
		"ko", "ko-KR",
		"pa", "pa-IN", "pa-PK",
		"ta", "ta-IN", "ta-LK",
		"te", "te-IN",
		"tr", "tr-TR",
		"vi", "vi-VN",
		"mr", "mr-IN",
		"jv", "jv-ID",
		"pl", "pl-PL",
		"nl", "nl-NL",
		"sv", "sv-SE",
		"no", "no-NO",
		"da", "da-DK",
		"fi", "fi-FI",
		"th", "th-TH",
		"el", "el-GR",
		"he", "he-IL",
		"fa", "fa-IR",
		"uk", "uk-UA",
		"ro", "ro-RO",
		"hu", "hu-HU",
		"cs", "cs-CZ",
		"sk", "sk-SK",
		"id", "id-ID",
		"ms", "ms-MY",
	}

	// Dynamic flag emoji generator
	flagEmoji := func(countryCode string) string {
		if len(countryCode) != 2 {
			return ""
		}
		cc := strings.ToUpper(countryCode)
		return string([]rune{
			0x1F1E6 + rune(cc[0]) - 'A',
			0x1F1E6 + rune(cc[1]) - 'A',
		})
	}

	fmt.Printf("\n🔍 Transios Search Results for %q:\n\n", args[0])
	fmt.Printf("%-8s | %-20s | %-20s | %-12s | %s\n", "Code", "English Name", "Native Name", "Region", "Flag")
	fmt.Println(strings.Repeat("-", 76))

	found := false

	for _, loc := range locales {
		tag, err := language.Parse(loc)
		if err != nil {
			continue
		}

		base, _ := tag.Base()
		region, _ := tag.Region()

		engName := display.English.Languages().Name(tag)
		nativeName := display.Self.Name(tag)

		regionCode := region.String()
		if regionCode == "" || regionCode == "ZZ" {
			regionCode = baseRegions[base.String()]
		}

		flag := flagEmoji(regionCode)

		// Match search query against English name, Native name, or code
		if strings.Contains(strings.ToLower(engName), query) ||
			strings.Contains(strings.ToLower(nativeName), query) ||
			strings.Contains(strings.ToLower(tag.String()), query) {
			
			displayRegion := region.String()
			if displayRegion == "" || displayRegion == "ZZ" {
				displayRegion = "Default (" + regionCode + ")"
			}

			fmt.Printf("%-8s | %-20s | %-20s | %-12s | %s\n", tag.String(), engName, nativeName, displayRegion, flag)
			found = true
		}
	}

	if !found {
		fmt.Println("No matching languages found.")
	} else {
		fmt.Println(strings.Repeat("-", 76))
		fmt.Println("\n💡 Add your desired code (e.g. \"es-ES\") to your SUPPORTED_LOCALES list in your .env file!")
	}
	fmt.Println()

	return nil
}
