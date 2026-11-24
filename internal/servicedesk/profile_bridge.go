package servicedesk

import (
	"context"

	sdp "github.com/serialq7ic4/ixf-toolbox/pkg/servicedeskprofile"
)

type APIError = sdp.APIError
type Service = sdp.Service
type Lading = sdp.Lading
type EditTarget = sdp.EditTarget
type Choice = sdp.Choice
type TableColumn = sdp.TableColumn
type Condition = sdp.Condition
type ConditionSet = sdp.ConditionSet
type Field = sdp.Field
type Template = sdp.Template
type Overlay = sdp.Overlay
type Routing = sdp.Routing
type OverlayTarget = sdp.OverlayTarget
type Baseline = sdp.Baseline
type BaselineField = sdp.BaselineField
type BaselineColumn = sdp.BaselineColumn
type FieldMatcher = sdp.FieldMatcher
type ColumnMatcher = sdp.ColumnMatcher
type OverlayField = sdp.OverlayField
type OverlayColumn = sdp.OverlayColumn
type ResolvedOverlay = sdp.ResolvedOverlay
type ResolvedField = sdp.ResolvedField
type ResolvedColumn = sdp.ResolvedColumn
type DriftReport = sdp.DriftReport
type DriftChange = sdp.DriftChange
type InspectRequest = sdp.InspectRequest
type ProfileContext = sdp.ProfileContext
type ResolvedTarget = sdp.ResolvedTarget
type InputColumn = sdp.InputColumn
type InputField = sdp.InputField
type InputContract = sdp.InputContract
type ResolvedProfile = sdp.ResolvedProfile
type ResolvedContract = sdp.ResolvedContract

const (
	OverlaySchemaVersion = sdp.OverlaySchemaVersion
	ContractVersion      = sdp.ContractVersion
	driftNonDestructive  = sdp.DriftNonDestructive
	driftDestructive     = sdp.DriftDestructive
)

var (
	ParseEditURL          = sdp.ParseEditURL
	IsValidProfileName    = sdp.IsValidProfileName
	DecodeOverlay         = sdp.DecodeOverlay
	ValidateOverlaySchema = sdp.ValidateOverlaySchema
	DiscoverOverlay       = sdp.DiscoverOverlay
	ValidateOverlay       = sdp.ValidateOverlay
	CompareBaseline       = sdp.CompareBaseline
	ResolveOverlay        = sdp.ResolveOverlay
)

func LoadTemplate(ctx context.Context, client *Client, target EditTarget) (Template, error) {
	return sdp.LoadTemplate(ctx, client, target)
}

func Inspect(ctx context.Context, client *Client, request InspectRequest) (ResolvedContract, error) {
	return sdp.Inspect(ctx, client, request)
}

func resolveProfileInputContract(fields []Field, overlay ResolvedOverlay) (InputContract, error) {
	return sdp.ResolveProfileInputContract(fields, overlay)
}

func projectProfileDrift(report DriftReport, fields []Field, profile InputContract) (DriftReport, error) {
	return sdp.ProjectProfileDrift(report, fields, profile)
}

func tableFieldType(fieldType string) bool  { return sdp.IsTableFieldType(fieldType) }
func preferredFieldName(field Field) string { return sdp.PreferredFieldName(field) }

func overlayRequired(required *bool) bool {
	return required != nil && *required
}
