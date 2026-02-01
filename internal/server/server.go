package server

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/ollama/ollama/api"
	"google.golang.org/genai"

	pb "arian-receipts/internal/gen/arian/v1"
)

const (
	maxImageSize = 5 << 20         // 5MB
	timeout      = 5 * time.Minute // it can take a while
)

const prompt = `Look at this receipt image and extract all information as JSON.

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
  "total": 51.61
}

Rules:
- "raw" is exactly as printed on receipt
- "name" is your best guess at the full product name
- "qty" defaults to 1.0 if not specified
- "unit_price" is price per unit
- "currency" is 3-letter ISO code
- "date" is YYYY-MM-DD if visible, otherwise null
- Use null for values you cannot read
- Skip items with $0.00 price`

type Server struct {
	pb.UnimplementedReceiptOCRServiceServer
	ollama *api.Client
	gemini *genai.Client
	model  string
	log    *log.Logger
}

func New(ollama *api.Client, gemini *genai.Client, model string, logger *log.Logger) *Server {
	return &Server{ollama: ollama, gemini: gemini, model: model, log: logger}
}

func (s *Server) ParseReceipt(ctx context.Context, req *pb.ParseReceiptRequest) (*pb.ParseReceiptResponse, error) {
	imageSize := len(req.ImageData)

	if imageSize == 0 {
		return errorResponse(pb.OCRErrorCode_OCR_ERROR_INVALID_IMAGE, "empty image"), nil
	}
	if imageSize > maxImageSize {
		return errorResponse(pb.OCRErrorCode_OCR_ERROR_INVALID_IMAGE, "image exceeds 5MB limit"), nil
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	contentType := cmp.Or(req.ContentType, "image/jpeg")
	raw, err := s.callModel(ctx, req.ImageData, contentType)
	if err != nil {
		s.log.Error("model call failed", "err", err)
		if ctx.Err() == context.DeadlineExceeded {
			return errorResponse(pb.OCRErrorCode_OCR_ERROR_TIMEOUT, "inference timeout"), nil
		}
		return errorResponse(pb.OCRErrorCode_OCR_ERROR_MODEL_ERROR, err.Error()), nil
	}

	receipt, err := parseResponse(raw)
	if err != nil {
		s.log.Warn("parse failed", "err", err)
		return errorResponse(pb.OCRErrorCode_OCR_ERROR_PARSE_FAILED, err.Error()), nil
	}

	receipt.Confidence = calcConfidence(receipt)

	s.log.Info("parsed receipt", "items", len(receipt.Items))

	return &pb.ParseReceiptResponse{Success: true, Data: receipt}, nil
}

func (s *Server) Health(ctx context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
	if s.ollama != nil {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		_, err := s.ollama.List(ctx)
		if err != nil {
			s.log.Warn("ollama health check failed", "err", err)
			return &pb.HealthResponse{Status: "unhealthy", Model: s.model}, nil
		}
	}

	return &pb.HealthResponse{Status: "ok", Model: s.model}, nil
}

func (s *Server) callModel(ctx context.Context, imageData []byte, contentType string) (string, error) {
	if s.gemini != nil {
		return s.callGemini(ctx, imageData, contentType)
	}
	return s.callOllama(ctx, imageData)
}

func (s *Server) callOllama(ctx context.Context, imageData []byte) (string, error) {
	stream := false
	req := &api.GenerateRequest{
		Model:  s.model,
		Prompt: prompt,
		Images: []api.ImageData{imageData},
		Stream: &stream,
	}

	var result string
	err := s.ollama.Generate(ctx, req, func(resp api.GenerateResponse) error {
		result = resp.Response
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("ollama: %w", err)
	}

	return result, nil
}

func (s *Server) callGemini(ctx context.Context, imageData []byte, contentType string) (string, error) {
	parts := []*genai.Part{
		{Text: prompt},
		{InlineData: &genai.Blob{MIMEType: contentType, Data: imageData}},
	}

	resp, err := s.gemini.Models.GenerateContent(ctx, s.model, []*genai.Content{{Parts: parts}}, nil)
	if err != nil {
		return "", fmt.Errorf("gemini: %w", err)
	}

	return resp.Text(), nil
}

// json types for parsing model output

type parsedJSON struct {
	Merchant string     `json:"merchant"`
	Date     *string    `json:"date"`
	Currency string     `json:"currency"`
	Items    []itemJSON `json:"items"`
	Subtotal *float64   `json:"subtotal"`
	Tax      *float64   `json:"tax"`
	Total    *float64   `json:"total"`
}

type itemJSON struct {
	Raw       string   `json:"raw"`
	Name      string   `json:"name"`
	Qty       *float64 `json:"qty"`
	UnitPrice *float64 `json:"unit_price"`
}

func parseResponse(raw string) (*pb.ParsedReceipt, error) {
	jsonStr := extractJSON(raw)

	var p parsedJSON
	if err := json.Unmarshal([]byte(jsonStr), &p); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	receipt := &pb.ParsedReceipt{}

	if p.Merchant != "" {
		receipt.Merchant = &p.Merchant
	}
	if p.Date != nil {
		receipt.Date = p.Date
	}
	if p.Currency != "" {
		receipt.Currency = &p.Currency
	}
	if p.Subtotal != nil {
		receipt.Subtotal = p.Subtotal
	}
	if p.Tax != nil {
		receipt.Tax = p.Tax
	}
	if p.Total != nil {
		receipt.Total = p.Total
	}

	for _, item := range p.Items {
		pbItem := &pb.ParsedItem{Raw: item.Raw}
		if item.Name != "" {
			pbItem.Name = &item.Name
		}
		if item.Qty != nil {
			pbItem.Qty = *item.Qty
		} else {
			pbItem.Qty = 1.0
		}
		if item.UnitPrice != nil {
			pbItem.UnitPrice = *item.UnitPrice
		}
		receipt.Items = append(receipt.Items, pbItem)
	}

	return receipt, nil
}

var codeBlockRe = regexp.MustCompile("```(?:json)?\\s*")

func extractJSON(s string) string {
	s = codeBlockRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "```", "")
	return strings.TrimSpace(s)
}

func calcConfidence(r *pb.ParsedReceipt) float32 {
	score := float32(0.5)

	if r.Merchant != nil {
		score += 0.1
	}
	if r.Date != nil {
		score += 0.1
	}
	if r.Total != nil {
		score += 0.1
	}
	if len(r.Items) > 0 {
		score += 0.1
	}

	// bonus if items sum approximately equals subtotal
	if r.Subtotal != nil && len(r.Items) > 0 {
		var sum float64
		for _, item := range r.Items {
			sum += item.Qty * item.UnitPrice
		}
		diff := math.Abs(sum - *r.Subtotal)
		withinTolerance := diff <= *r.Subtotal*0.05
		if withinTolerance {
			score += 0.1
		}
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

func errorResponse(code pb.OCRErrorCode, msg string) *pb.ParseReceiptResponse {
	return &pb.ParseReceiptResponse{
		Success: false,
		Error:   &pb.OCRError{Code: code, Message: msg},
	}
}

func ptrOr[T any](p *T, fallback T) T {
	if p != nil {
		return *p
	}
	return fallback
}
