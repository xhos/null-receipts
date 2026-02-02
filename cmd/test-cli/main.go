// TODO: make it re-use the actual prompt and logic from the main pipeline

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/image/draw"
)

const (
	ollamaURL = "http://localhost:11434/api/generate"
	model     = "qwen2.5vl:3b"
	maxWidth  = 800 // resize images wider than this
)

type Item struct {
	Raw       string   `json:"raw"`
	Name      string   `json:"name"`
	Qty       *float64 `json:"qty"`
	UnitPrice *float64 `json:"unit_price"`
}

type Receipt struct {
	Merchant   string   `json:"merchant"`
	Date       *string  `json:"date"`
	Currency   string   `json:"currency"`
	Items      []Item   `json:"items"`
	Subtotal   *float64 `json:"subtotal"`
	Tax        *float64 `json:"tax"`
	Total      *float64 `json:"total"`
	Confidence *float64 `json:"confidence"`
}

type OllamaRequest struct {
	Model  string   `json:"model"`
	Prompt string   `json:"prompt"`
	Images []string `json:"images"`
	Stream bool     `json:"stream"`
}

type OllamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

type Result struct {
	File    string   `json:"file"`
	Success bool     `json:"success"`
	Receipt *Receipt `json:"receipt,omitempty"`
	Error   string   `json:"error,omitempty"`
	RawResp string   `json:"raw_response,omitempty"`
}

var prompt = `Look at this receipt image and extract all information as JSON.

Return ONLY valid JSON in this exact format (no markdown, no backticks, no explanation):
{
  "merchant": "Store Name",
  "date": "YYYY-MM-DD",
  "currency": "CAD",
  "items": [
    {"raw": "KIRKLAND ORG EGGS 2DZ", "name": "Organic Eggs 2 Dozen", "qty": 1.0, "unit_price": 8.99}
  ],
  "subtotal": 45.67,
  "tax": 5.94,
  "total": 51.61,
  "confidence": 0.92
}

Rules:
- "raw" is exactly as printed on receipt (e.g., "PC SFT CKIE MCAD")
- "name" is your best guess at the full product name (e.g., "PC Soft Cookie Macadamia")
- "qty" is a float (e.g., 1.0, 1.5, 0.5) - default to 1.0 if not specified
- "unit_price" is price per unit as a number
- "currency" is the 3-letter code (CAD, USD, etc.) - infer from $ symbols and store location
- "confidence" is 0.0-1.0 indicating how confident you are in the overall extraction
- "date" should be YYYY-MM-DD format if visible, otherwise null
- Use null for any values you cannot read
- Do NOT include promotional items with $0.00 price`

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <image_path> [image_path2] ...")
		fmt.Println("       go run main.go *.jpg")
		os.Exit(1)
	}

	var results []Result

	for _, imagePath := range os.Args[1:] {
		result := processImage(imagePath)
		results = append(results, result)

		status := "✓"
		if !result.Success {
			status = "✗"
		}
		fmt.Fprintf(os.Stderr, "%s %s\n", status, filepath.Base(imagePath))
	}

	output, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(output))
}

func processImage(imagePath string) Result {
	result := Result{File: filepath.Base(imagePath)}

	b64Image, err := loadAndResizeImage(imagePath)
	if err != nil {
		result.Error = fmt.Sprintf("failed to process image: %v", err)
		return result
	}

	req := OllamaRequest{
		Model:  model,
		Prompt: prompt,
		Images: []string{b64Image},
		Stream: false,
	}

	reqBody, _ := json.Marshal(req)

	resp, err := http.Post(ollamaURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		result.Error = fmt.Sprintf("ollama request failed: %v", err)
		return result
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("ollama error (%d): %s", resp.StatusCode, string(body))
		return result
	}

	var ollamaResp OllamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		result.Error = fmt.Sprintf("failed to parse ollama response: %v", err)
		return result
	}

	// extract JSON from response (strip markdown code fences if present)
	jsonStr := extractJSON(ollamaResp.Response)
	result.RawResp = ollamaResp.Response

	// parse and validate against schema
	var receipt Receipt
	if err := json.Unmarshal([]byte(jsonStr), &receipt); err != nil {
		result.Error = fmt.Sprintf("invalid JSON: %v", err)
		return result
	}

	// validate required fields
	if err := validateReceipt(&receipt); err != nil {
		result.Error = fmt.Sprintf("schema validation failed: %v", err)
		return result
	}

	result.Success = true
	result.Receipt = &receipt
	result.RawResp = ""
	return result
}

func loadAndResizeImage(imagePath string) (string, error) {
	f, err := os.Open(imagePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width > maxWidth {
		newHeight := int(float64(height) * float64(maxWidth) / float64(width))
		resized := image.NewRGBA(image.Rect(0, 0, maxWidth, newHeight))
		draw.CatmullRom.Scale(resized, resized.Bounds(), img, bounds, draw.Over, nil)
		img = resized
		fmt.Fprintf(os.Stderr, "  resized %dx%d -> %dx%d\n", width, height, maxWidth, newHeight)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return "", fmt.Errorf("encode: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func extractJSON(s string) string {
	re := regexp.MustCompile("```(?:json)?\\s*")
	s = re.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "```", "")
	s = strings.TrimSpace(s)
	return s
}

func validateReceipt(r *Receipt) error {
	if r.Merchant == "" {
		return fmt.Errorf("merchant is required")
	}
	if r.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	if r.Total == nil {
		return fmt.Errorf("total is required")
	}
	if len(r.Items) == 0 {
		return fmt.Errorf("at least one item is required")
	}
	for i, item := range r.Items {
		if item.Raw == "" {
			return fmt.Errorf("item[%d].raw is required", i)
		}
		if item.Name == "" {
			return fmt.Errorf("item[%d].name is required", i)
		}
		if item.UnitPrice == nil {
			return fmt.Errorf("item[%d].unit_price is required", i)
		}
	}
	return nil
}
