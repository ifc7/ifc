package client

import (
	"context"
	"io"
)

type ClientWithResponsesIfc interface {
	GetHealthWithResponse(ctx context.Context, reqEditors ...RequestEditorFn) (*GetHealthResponse, error)
	GetInterfaceByPathWithResponse(ctx context.Context, ownerScope GetInterfaceByPathParamsOwnerScope, ownerName string, interfaceName string, params *GetInterfaceByPathParams, reqEditors ...RequestEditorFn) (*GetInterfaceByPathResponse, error)
	ListInterfacesWithResponse(ctx context.Context, reqEditors ...RequestEditorFn) (*ListInterfacesResponse, error)
	CreateInterfaceWithBodyWithResponse(ctx context.Context, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*CreateInterfaceResponse, error)
	CreateInterfaceWithResponse(ctx context.Context, body CreateInterfaceJSONRequestBody, reqEditors ...RequestEditorFn) (*CreateInterfaceResponse, error)
	DeleteInterfaceWithResponse(ctx context.Context, interfaceId InterfaceId, reqEditors ...RequestEditorFn) (*DeleteInterfaceResponse, error)
	GetInterfaceWithResponse(ctx context.Context, interfaceId InterfaceId, params *GetInterfaceParams, reqEditors ...RequestEditorFn) (*GetInterfaceResponse, error)
	ListInterfaceReleasesWithResponse(ctx context.Context, interfaceId InterfaceId, reqEditors ...RequestEditorFn) (*ListInterfaceReleasesResponse, error)
	CreateInterfaceReleaseWithBodyWithResponse(ctx context.Context, interfaceId InterfaceId, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*CreateInterfaceReleaseResponse, error)
	CreateInterfaceReleaseWithResponse(ctx context.Context, interfaceId InterfaceId, body CreateInterfaceReleaseJSONRequestBody, reqEditors ...RequestEditorFn) (*CreateInterfaceReleaseResponse, error)
	DeleteInterfaceReleaseWithResponse(ctx context.Context, interfaceId InterfaceId, semVer SemanticVersion, reqEditors ...RequestEditorFn) (*DeleteInterfaceReleaseResponse, error)
	GetInterfaceReleaseWithResponse(ctx context.Context, interfaceId InterfaceId, semVer SemanticVersion, reqEditors ...RequestEditorFn) (*GetInterfaceReleaseResponse, error)
	ListInterfaceRevisionsWithResponse(ctx context.Context, interfaceId InterfaceId, reqEditors ...RequestEditorFn) (*ListInterfaceRevisionsResponse, error)
	CreateInterfaceRevisionWithBodyWithResponse(ctx context.Context, interfaceId InterfaceId, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*CreateInterfaceRevisionResponse, error)
	CreateInterfaceRevisionWithResponse(ctx context.Context, interfaceId InterfaceId, body CreateInterfaceRevisionJSONRequestBody, reqEditors ...RequestEditorFn) (*CreateInterfaceRevisionResponse, error)
	GetInterfaceRevisionWithResponse(ctx context.Context, interfaceId InterfaceId, revisionId InterfaceRevisionId, reqEditors ...RequestEditorFn) (*GetInterfaceRevisionResponse, error)
	ListOrganizationsWithResponse(ctx context.Context, reqEditors ...RequestEditorFn) (*ListOrganizationsResponse, error)
	CreateOrganizationWithBodyWithResponse(ctx context.Context, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*CreateOrganizationResponse, error)
	CreateOrganizationWithResponse(ctx context.Context, body CreateOrganizationJSONRequestBody, reqEditors ...RequestEditorFn) (*CreateOrganizationResponse, error)
	ListOrganizationMembersWithResponse(ctx context.Context, organizationId OrganizationId, reqEditors ...RequestEditorFn) (*ListOrganizationMembersResponse, error)
	AddOrganizationMemberWithBodyWithResponse(ctx context.Context, organizationId OrganizationId, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*AddOrganizationMemberResponse, error)
	AddOrganizationMemberWithResponse(ctx context.Context, organizationId OrganizationId, body AddOrganizationMemberJSONRequestBody, reqEditors ...RequestEditorFn) (*AddOrganizationMemberResponse, error)
	DeleteOrganizationMemberWithResponse(ctx context.Context, organizationId OrganizationId, userId UserId, reqEditors ...RequestEditorFn) (*DeleteOrganizationMemberResponse, error)
	GetOrganizationMemberWithResponse(ctx context.Context, organizationId OrganizationId, userId UserId, reqEditors ...RequestEditorFn) (*GetOrganizationMemberResponse, error)
	UpdateOrganizationMemberWithBodyWithResponse(ctx context.Context, organizationId OrganizationId, userId UserId, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*UpdateOrganizationMemberResponse, error)
	UpdateOrganizationMemberWithResponse(ctx context.Context, organizationId OrganizationId, userId UserId, body UpdateOrganizationMemberJSONRequestBody, reqEditors ...RequestEditorFn) (*UpdateOrganizationMemberResponse, error)
	GetOrganizationSettingsWithResponse(ctx context.Context, organizationId OrganizationId, reqEditors ...RequestEditorFn) (*GetOrganizationSettingsResponse, error)
	UpdateOrganizationSettingsWithBodyWithResponse(ctx context.Context, organizationId OrganizationId, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*UpdateOrganizationSettingsResponse, error)
	UpdateOrganizationSettingsWithResponse(ctx context.Context, organizationId OrganizationId, body UpdateOrganizationSettingsJSONRequestBody, reqEditors ...RequestEditorFn) (*UpdateOrganizationSettingsResponse, error)
	GetUserSettingsWithResponse(ctx context.Context, userId UserId, reqEditors ...RequestEditorFn) (*GetUserSettingsResponse, error)
	UpdateUserSettingsWithBodyWithResponse(ctx context.Context, userId UserId, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*UpdateUserSettingsResponse, error)
	UpdateUserSettingsWithResponse(ctx context.Context, userId UserId, body UpdateUserSettingsJSONRequestBody, reqEditors ...RequestEditorFn) (*UpdateUserSettingsResponse, error)
}
