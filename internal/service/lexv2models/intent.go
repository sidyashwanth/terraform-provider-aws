// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lexv2models

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lexmodelsv2"
	awstypes "github.com/aws/aws-sdk-go-v2/service/lexmodelsv2/types"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-provider-aws/internal/create"
	"github.com/hashicorp/terraform-provider-aws/internal/framework"
	"github.com/hashicorp/terraform-provider-aws/internal/framework/flex"
	fwtypes "github.com/hashicorp/terraform-provider-aws/internal/framework/types"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/names"
)

// @FrameworkResource(name="Intent")
func newResourceIntent(_ context.Context) (resource.ResourceWithConfigure, error) {
	r := &resourceIntent{}

	r.SetDefaultCreateTimeout(30 * time.Minute)
	r.SetDefaultUpdateTimeout(30 * time.Minute)
	r.SetDefaultDeleteTimeout(30 * time.Minute)

	return r, nil
}

const (
	ResNameIntent = "Intent"
)

type resourceIntent struct {
	framework.ResourceWithConfigure
	framework.WithImportByID
	framework.WithTimeouts
}

func (r *resourceIntent) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "aws_lexv2models_intent"
}

func (r *resourceIntent) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	// building blocks for the schema
	messageNBO := schema.NestedBlockObject{
		Blocks: map[string]schema.Block{
			"custom_playload": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				CustomType: fwtypes.NewListNestedObjectTypeOf[CustomPayload](ctx),
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"value": schema.StringAttribute{
							Required: true,
						},
					},
				},
			},
			"image_response_card": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				CustomType: fwtypes.NewListNestedObjectTypeOf[ImageResponseCard](ctx),
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"image_url": schema.StringAttribute{
							Optional: true,
						},
						"subtitle": schema.StringAttribute{
							Optional: true,
						},
						"title": schema.StringAttribute{
							Required: true,
						},
					},
					Blocks: map[string]schema.Block{
						"button": schema.ListNestedBlock{
							CustomType: fwtypes.NewListNestedObjectTypeOf[Button](ctx),
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"text": schema.StringAttribute{
										Required: true,
									},
									"value": schema.StringAttribute{
										Required: true,
									},
								},
							},
						},
					},
				},
			},
			"plain_text_message": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				CustomType: fwtypes.NewListNestedObjectTypeOf[PlainTextMessage](ctx),
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"value": schema.StringAttribute{
							Required: true,
						},
					},
				},
			},
			"ssml_message": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				CustomType: fwtypes.NewListNestedObjectTypeOf[SSMLMessage](ctx),
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"value": schema.StringAttribute{
							Required: true,
						},
					},
				},
			},
		},
	}
	messageGroupLNB := schema.ListNestedBlock{
		Validators: []validator.List{
			listvalidator.SizeAtLeast(1),
		},
		CustomType: fwtypes.NewListNestedObjectTypeOf[MessageGroup](ctx),
		NestedObject: schema.NestedBlockObject{
			Blocks: map[string]schema.Block{
				"message": schema.ListNestedBlock{
					Validators: []validator.List{
						listvalidator.SizeBetween(1, 1),
					},
					CustomType:   fwtypes.NewListNestedObjectTypeOf[Message](ctx),
					NestedObject: messageNBO,
				},
				"variations": schema.ListNestedBlock{
					CustomType:   fwtypes.NewListNestedObjectTypeOf[Message](ctx),
					NestedObject: messageNBO,
				},
			},
		},
	}
	// O for "Object"
	slotValueOverrideO := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"shape": types.StringType,
			"value": types.ObjectType{
				AttrTypes: map[string]attr.Type{
					"interpreted_value": types.StringType,
				},
			},
		},
	}

	// This recursive type blows up autoflex and potentially schema.
	// In order to continue work, we are intentionally leaving this out for now
	// and will need to return to it when needed.

	//slotValueOverrideO.AttrTypes["values"] = slotValueOverrideO

	dialogStateNBO := schema.NestedBlockObject{
		Attributes: map[string]schema.Attribute{
			"session_attributes": schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"dialog_action": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				CustomType: fwtypes.NewListNestedObjectTypeOf[DialogAction](ctx),
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Required:   true,
							CustomType: fwtypes.StringEnumType[awstypes.DialogActionType](),
						},
						"slot_to_elicit": schema.StringAttribute{
							Optional: true,
						},
						"suppress_next_message": schema.StringAttribute{
							Optional: true,
						},
					},
				},
			},
			"intent": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				CustomType: fwtypes.NewListNestedObjectTypeOf[IntentOverride](ctx),
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Optional: true,
						},
						"slots": schema.MapAttribute{
							Optional:    true,
							ElementType: slotValueOverrideO,
						},
					},
				},
			},
		},
	}
	responseSpecificationLNB := schema.ListNestedBlock{
		Validators: []validator.List{
			listvalidator.SizeAtMost(1),
		},
		CustomType: fwtypes.NewListNestedObjectTypeOf[ResponseSpecification](ctx),
		NestedObject: schema.NestedBlockObject{
			Attributes: map[string]schema.Attribute{
				"allow_interrupt": schema.BoolAttribute{
					Optional: true,
				},
			},
			Blocks: map[string]schema.Block{
				"message_group": messageGroupLNB,
			},
		},
	}
	conditionalSpecificationLNB := schema.ListNestedBlock{
		Validators: []validator.List{
			listvalidator.SizeAtMost(1),
		},
		CustomType: fwtypes.NewListNestedObjectTypeOf[ConditionalSpecification](ctx),
		NestedObject: schema.NestedBlockObject{
			Attributes: map[string]schema.Attribute{
				"active": schema.BoolAttribute{
					Required: true,
				},
			},
			Blocks: map[string]schema.Block{
				"conditional_branch": schema.ListNestedBlock{
					Validators: []validator.List{
						listvalidator.SizeAtLeast(1),
					},
					CustomType: fwtypes.NewListNestedObjectTypeOf[ConditionalBranch](ctx),
					NestedObject: schema.NestedBlockObject{
						Attributes: map[string]schema.Attribute{
							"name": schema.StringAttribute{
								Required: true,
							},
						},
						Blocks: map[string]schema.Block{
							"condition": schema.ListNestedBlock{
								Validators: []validator.List{
									listvalidator.SizeBetween(1, 1),
								},
								CustomType: fwtypes.NewListNestedObjectTypeOf[Condition](ctx),
								NestedObject: schema.NestedBlockObject{
									Attributes: map[string]schema.Attribute{
										"expression_string": schema.StringAttribute{
											Required: true,
										},
									},
								},
							},
							"next_step": schema.ListNestedBlock{
								Validators: []validator.List{
									listvalidator.SizeBetween(1, 1),
								},
								CustomType:   fwtypes.NewListNestedObjectTypeOf[DialogState](ctx),
								NestedObject: dialogStateNBO,
							},
							"response": responseSpecificationLNB,
						},
					},
				},
				"default_branch": schema.ListNestedBlock{
					Validators: []validator.List{
						listvalidator.SizeBetween(1, 1),
					},
					CustomType: fwtypes.NewListNestedObjectTypeOf[DefaultConditionalBranch](ctx),
					NestedObject: schema.NestedBlockObject{
						Blocks: map[string]schema.Block{
							"next_step": schema.ListNestedBlock{
								Validators: []validator.List{
									listvalidator.SizeAtMost(1),
								},
								CustomType:   fwtypes.NewListNestedObjectTypeOf[DialogState](ctx),
								NestedObject: dialogStateNBO,
							},
							"response": responseSpecificationLNB,
						},
					},
				},
			},
		},
	}
	failureSuccessTimeoutNBO := schema.NestedBlockObject{
		Blocks: map[string]schema.Block{
			"failure_conditional": conditionalSpecificationLNB,
			"failure_next_step": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				CustomType:   fwtypes.NewListNestedObjectTypeOf[DialogState](ctx),
				NestedObject: dialogStateNBO,
			},
			"failure_response":    responseSpecificationLNB,
			"success_conditional": conditionalSpecificationLNB,
			"success_next_step": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				CustomType:   fwtypes.NewListNestedObjectTypeOf[DialogState](ctx),
				NestedObject: dialogStateNBO,
			},
			"success_response":    responseSpecificationLNB,
			"timeout_conditional": conditionalSpecificationLNB,
			"timeout_next_step": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				CustomType:   fwtypes.NewListNestedObjectTypeOf[DialogState](ctx),
				NestedObject: dialogStateNBO,
			},
			"timeout_response": responseSpecificationLNB,
		},
	}

	// start of schema proper
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"bot_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"bot_version": schema.StringAttribute{
				Required: true,
			},
			"creation_date_time": schema.StringAttribute{
				Computed:   true,
				CustomType: fwtypes.TimestampType,
			},
			"description": schema.StringAttribute{
				Optional: true,
			},
			"id": framework.IDAttribute(),
			"intent_id": schema.StringAttribute{
				Computed: true,
			},
			"last_updated_date_time": schema.StringAttribute{
				Computed:   true,
				CustomType: fwtypes.TimestampType,
			},
			"locale_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"parent_intent_signature": schema.StringAttribute{
				Optional: true,
			},
		},
		Blocks: map[string]schema.Block{
			"dialog_code_hook": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				CustomType: fwtypes.NewListNestedObjectTypeOf[DialogCodeHookSettings](ctx),
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"enabled": schema.BoolAttribute{
							Required: true,
						},
					},
				},
			},
			"fulfillment_code_hook": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				CustomType: fwtypes.NewListNestedObjectTypeOf[FulfillmentCodeHookSettings](ctx),
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"enabled": schema.BoolAttribute{
							Required: true,
						},
						"active": schema.BoolAttribute{
							Computed: true,
						},
					},
					Blocks: map[string]schema.Block{
						"fulfillment_updates_specification": schema.ListNestedBlock{
							Validators: []validator.List{
								listvalidator.SizeAtMost(1),
							},
							CustomType: fwtypes.NewListNestedObjectTypeOf[FulfillmentUpdatesSpecification](ctx),
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"active": schema.BoolAttribute{
										Required: true,
									},
									"timeout_in_seconds": schema.Int64Attribute{
										Optional: true,
									},
								},
								Blocks: map[string]schema.Block{
									"start_response": schema.ListNestedBlock{
										Validators: []validator.List{
											listvalidator.SizeAtMost(1),
										},
										CustomType: fwtypes.NewListNestedObjectTypeOf[FulfillmentStartResponseSpecification](ctx),
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"allow_interrupt": schema.BoolAttribute{
													Optional: true,
												},
												"delay_in_seconds": schema.Int64Attribute{
													Optional: true,
												},
											},
											Blocks: map[string]schema.Block{
												"message_group": schema.ListNestedBlock{
													Validators: []validator.List{
														listvalidator.SizeBetween(1, 5),
													},
													CustomType: fwtypes.NewListNestedObjectTypeOf[MessageGroup](ctx),
													NestedObject: schema.NestedBlockObject{
														Blocks: map[string]schema.Block{
															"message": schema.ListNestedBlock{
																Validators: []validator.List{
																	listvalidator.SizeBetween(1, 1),
																},
																CustomType:   fwtypes.NewListNestedObjectTypeOf[Message](ctx),
																NestedObject: messageNBO,
															},
															"variations": schema.ListNestedBlock{
																CustomType:   fwtypes.NewListNestedObjectTypeOf[Message](ctx),
																NestedObject: messageNBO,
															},
														},
													},
												},
											},
										},
									},
									"update_response": schema.ListNestedBlock{
										Validators: []validator.List{
											listvalidator.SizeAtMost(1),
										},
										CustomType: fwtypes.NewListNestedObjectTypeOf[FulfillmentUpdateResponseSpecification](ctx),
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"allow_interrupt": schema.BoolAttribute{
													Optional: true,
												},
												"frequency_in_seconds": schema.Int64Attribute{
													Required: true,
												},
											},
											Blocks: map[string]schema.Block{
												"message_group": messageGroupLNB,
											},
										},
									},
								},
							},
						},
						"post_fulfillment_status_specification": schema.ListNestedBlock{
							Validators: []validator.List{
								listvalidator.SizeAtMost(1),
							},
							CustomType:   fwtypes.NewListNestedObjectTypeOf[FailureSuccessTimeout](ctx),
							NestedObject: failureSuccessTimeoutNBO,
						},
					},
				},
			},
			"initial_response_setting": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				CustomType: fwtypes.NewListNestedObjectTypeOf[InitialResponseSetting](ctx),
				NestedObject: schema.NestedBlockObject{
					Blocks: map[string]schema.Block{
						"code_hook": schema.ListNestedBlock{
							Validators: []validator.List{
								listvalidator.SizeAtMost(1),
							},
							CustomType: fwtypes.NewListNestedObjectTypeOf[DialogCodeHookInvocationSetting](ctx),
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"active": schema.BoolAttribute{
										Required: true,
									},
									"enable_code_hook_invocation": schema.BoolAttribute{
										Required: true,
									},
									"invocation_label": schema.StringAttribute{
										Optional: true,
									},
								},
								Blocks: map[string]schema.Block{
									"post_code_hook_specification": schema.ListNestedBlock{
										Validators: []validator.List{
											listvalidator.SizeBetween(1, 1),
										},
										CustomType:   fwtypes.NewListNestedObjectTypeOf[FailureSuccessTimeout](ctx),
										NestedObject: failureSuccessTimeoutNBO,
									},
								},
							},
						},
						"conditional":      conditionalSpecificationLNB,
						"initial_response": responseSpecificationLNB,
						"next_step": schema.ListNestedBlock{
							Validators: []validator.List{
								listvalidator.SizeAtMost(1),
							},
							CustomType:   fwtypes.NewListNestedObjectTypeOf[DialogState](ctx),
							NestedObject: dialogStateNBO,
						},
					},
				},
			},
			"input_context": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(5),
				},
				CustomType: fwtypes.NewListNestedObjectTypeOf[InputContext](ctx),
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required: true,
						},
					},
				},
			},
			"closing_setting": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				CustomType: fwtypes.NewListNestedObjectTypeOf[IntentClosingSetting](ctx),
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"active": schema.BoolAttribute{
							Optional: true,
						},
					},
					Blocks: map[string]schema.Block{
						"closing_response": responseSpecificationLNB,
						"conditional":      conditionalSpecificationLNB,
						"next_step": schema.ListNestedBlock{
							Validators: []validator.List{
								listvalidator.SizeAtMost(1),
							},
							CustomType:   fwtypes.NewListNestedObjectTypeOf[DialogState](ctx),
							NestedObject: dialogStateNBO,
						},
					},
				},
			},
			"confirmation_setting": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				CustomType: fwtypes.NewListNestedObjectTypeOf[IntentConfirmationSetting](ctx),
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"active": schema.BoolAttribute{
							Optional: true,
						},
					},
					Blocks: map[string]schema.Block{
						"prompt_specification": schema.ListNestedBlock{
							Validators: []validator.List{
								listvalidator.SizeBetween(1, 1),
							},
							CustomType: fwtypes.NewListNestedObjectTypeOf[PromptSpecification](ctx),
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"allow_interrupt": schema.BoolAttribute{
										Optional: true,
									},
									"max_retries": schema.Int64Attribute{
										Required: true,
									},
									"message_selection_strategy": schema.StringAttribute{
										Optional:   true,
										CustomType: fwtypes.StringEnumType[awstypes.MessageSelectionStrategy](),
									},
									"prompt_attempts_specification": schema.MapAttribute{
										Optional: true,
										ElementType: types.ObjectType{
											AttrTypes: map[string]attr.Type{
												"allowed_input_types": types.ObjectType{
													AttrTypes: map[string]attr.Type{
														"allow_audio_input": types.BoolType,
														"allow_dtmf_input":  types.BoolType,
													},
												},
												"allow_interrupt": types.BoolType,
												"audio_and_dtmf_input_specification": types.ObjectType{
													AttrTypes: map[string]attr.Type{
														"start_timeout_ms": types.Int64Type,
														"audio_specification": types.ObjectType{
															AttrTypes: map[string]attr.Type{
																"end_timeout_ms": types.Int64Type,
																"max_length_ms":  types.Int64Type,
															},
														},
														"dtmf_specification": types.ObjectType{
															AttrTypes: map[string]attr.Type{
																"deletion_character": types.StringType,
																"end_character":      types.StringType,
																"end_timeout_ms":     types.Int64Type,
																"max_length":         types.Int64Type,
															},
														},
													},
												},
											},
										},
									},
								},
								Blocks: map[string]schema.Block{
									"message_group": messageGroupLNB,
								},
							},
						},
						"conditional": conditionalSpecificationLNB,
						"next_step": schema.ListNestedBlock{
							Validators: []validator.List{
								listvalidator.SizeAtMost(1),
							},
							CustomType:   fwtypes.NewListNestedObjectTypeOf[DialogState](ctx),
							NestedObject: dialogStateNBO,
						},
					},
				},
			},
			"kendra_configuration": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				CustomType: fwtypes.NewListNestedObjectTypeOf[KendraConfiguration](ctx),
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"index": schema.StringAttribute{
							Required: true,
						},
						"query_filter_string": schema.StringAttribute{
							Optional: true,
						},
						"query_filter_string_enabled": schema.BoolAttribute{
							Optional: true,
						},
					},
				},
			},
			"output_context": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(10),
				},
				CustomType: fwtypes.NewListNestedObjectTypeOf[OutputContext](ctx),
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required: true,
						},
						"time_to_live_in_seconds": schema.Int64Attribute{
							Required: true,
						},
						"turns_to_live": schema.Int64Attribute{
							Required: true,
						},
					},
				},
			},
			"sample_utterance": schema.ListNestedBlock{
				CustomType: fwtypes.NewListNestedObjectTypeOf[SampleUtterance](ctx),
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"utterance": schema.StringAttribute{
							Required: true,
						},
					},
				},
			},
			"slot_priority": schema.ListNestedBlock{
				CustomType: fwtypes.NewListNestedObjectTypeOf[SlotPriority](ctx),
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"priority": schema.Int64Attribute{
							Required: true,
						},
						"slot_id": schema.StringAttribute{
							Required: true,
						},
					},
				},
			},
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// todo:
//  x invalid result object
//  x get basic test working
//  x import/ID
//  - test all attributes
//  x tags (no tags on intents)
//  - slot_priority
//  x timeouts
//  - updates
//  x disappears

func (r *resourceIntent) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	conn := r.Meta().LexV2ModelsClient(ctx)

	var data ResourceIntentData
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := &lexmodelsv2.CreateIntentInput{}
	resp.Diagnostics.Append(flex.Expand(context.WithValue(ctx, flex.ResourcePrefix, ResNameIntent), &data, in)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := conn.CreateIntent(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.LexV2Models, create.ErrActionCreating, ResNameIntent, data.Name.String(), err),
			err.Error(),
		)
		return
	}
	if out == nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.LexV2Models, create.ErrActionCreating, ResNameIntent, data.Name.String(), nil),
			errors.New("empty output").Error(),
		)
		return
	}

	data.IntentID = flex.StringToFramework(ctx, out.IntentId)
	data.setID()

	intent, err := waitIntentNormal(ctx, conn, data.IntentID.ValueString(), data.BotID.ValueString(), data.BotVersion.ValueString(), data.LocaleID.ValueString(), r.CreateTimeout(ctx, data.Timeouts))
	if err != nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.LexV2Models, create.ErrActionWaitingForCreation, ResNameIntent, data.ID.String(), err),
			err.Error(),
		)
		return
	}

	// get some data from the intent
	var dataAfter ResourceIntentData
	resp.Diagnostics.Append(flex.Flatten(ctx, intent, &dataAfter)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// unknowns must be set to satisfy apply
	data.CreationDateTime = dataAfter.CreationDateTime
	data.LastUpdatedDateTime = dataAfter.LastUpdatedDateTime

	//if len(data.SlotPriority) > 0 {
	// update because SlotPriority can't be set on create
	//}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *resourceIntent) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	conn := r.Meta().LexV2ModelsClient(ctx)

	var state ResourceIntentData
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := state.InitFromID(); err != nil {
		resp.Diagnostics.AddError("parsing resource ID", err.Error())
		return
	}

	out, err := findIntentByIDs(ctx, conn, state.IntentID.ValueString(), state.BotID.ValueString(), state.BotVersion.ValueString(), state.LocaleID.ValueString())

	if tfresource.NotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}

	if err != nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.LexV2Models, create.ErrActionSetting, ResNameIntent, state.ID.String(), err),
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(flex.Flatten(context.WithValue(ctx, flex.ResourcePrefix, ResNameIntent), out, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *resourceIntent) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	conn := r.Meta().LexV2ModelsClient(ctx)

	var plan, state ResourceIntentData
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := &lexmodelsv2.UpdateIntentInput{
		BotId:      plan.BotID.ValueStringPointer(),
		BotVersion: plan.BotVersion.ValueStringPointer(),
		IntentId:   plan.IntentID.ValueStringPointer(),
		IntentName: plan.Name.ValueStringPointer(),
		LocaleId:   plan.LocaleID.ValueStringPointer(),
	}

	resp.Diagnostics.Append(flex.Expand(ctx, &plan, in)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := conn.UpdateIntent(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.LexV2Models, create.ErrActionUpdating, ResNameIntent, plan.ID.String(), err),
			err.Error(),
		)
		return
	}
	if out == nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.LexV2Models, create.ErrActionUpdating, ResNameIntent, plan.ID.String(), nil),
			errors.New("empty output").Error(),
		)
		return
	}

	plan.IntentID = flex.StringToFramework(ctx, out.IntentId)
	plan.setID()

	_, err = waitIntentNormal(ctx, conn, plan.IntentID.ValueString(), plan.BotID.ValueString(), plan.BotVersion.ValueString(), plan.LocaleID.ValueString(), r.UpdateTimeout(ctx, plan.Timeouts))
	if err != nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.LexV2Models, create.ErrActionWaitingForUpdate, ResNameIntent, plan.ID.String(), err),
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *resourceIntent) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	conn := r.Meta().LexV2ModelsClient(ctx)

	var state ResourceIntentData
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := &lexmodelsv2.DeleteIntentInput{
		IntentId:   aws.String(state.IntentID.ValueString()),
		BotId:      aws.String(state.BotID.ValueString()),
		BotVersion: aws.String(state.BotVersion.ValueString()),
		LocaleId:   aws.String(state.LocaleID.ValueString()),
	}

	_, err := conn.DeleteIntent(ctx, in)
	if err != nil {
		var nfe *awstypes.ResourceNotFoundException // lexv2models does not seem to use this approach like other services
		if errors.As(err, &nfe) {
			return
		}

		var pfe *awstypes.PreconditionFailedException // PreconditionFailedException: Failed to retrieve resource since it does not exist
		if errors.As(err, &pfe) {
			return
		}

		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.LexV2Models, create.ErrActionDeleting, ResNameIntent, state.ID.String(), err),
			err.Error(),
		)
		return
	}

	_, err = waitIntentDeleted(ctx, conn, state.IntentID.ValueString(), state.BotID.ValueString(), state.BotVersion.ValueString(), state.LocaleID.ValueString(), r.DeleteTimeout(ctx, state.Timeouts))
	if err != nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.LexV2Models, create.ErrActionWaitingForDeletion, ResNameIntent, state.ID.String(), err),
			err.Error(),
		)
		return
	}
}

func (model *ResourceIntentData) InitFromID() error {
	parts := strings.Split(model.ID.ValueString(), ":")
	if len(parts) != 4 {
		return fmt.Errorf("Unexpected format for import key (%s), use: IntentID:BotID:BotVersion:LocaleID", model.ID)
	}
	model.IntentID = types.StringValue(parts[0])
	model.BotID = types.StringValue(parts[1])
	model.BotVersion = types.StringValue(parts[2])
	model.LocaleID = types.StringValue(parts[3])

	return nil
}

func (model *ResourceIntentData) setID() {
	model.ID = types.StringValue(strings.Join([]string{
		model.IntentID.ValueString(),
		model.BotID.ValueString(),
		model.BotVersion.ValueString(),
		model.LocaleID.ValueString(),
	}, ":"))
}

const (
	statusNormal = "Normal"
)

func waitIntentNormal(ctx context.Context, conn *lexmodelsv2.Client, intentID, botID, botVersion, localeID string, timeout time.Duration) (*lexmodelsv2.DescribeIntentOutput, error) {
	stateConf := &retry.StateChangeConf{
		Pending:                   []string{},
		Target:                    []string{statusNormal},
		Refresh:                   statusIntent(ctx, conn, intentID, botID, botVersion, localeID),
		Timeout:                   timeout,
		MinTimeout:                5 * time.Second,
		NotFoundChecks:            20,
		ContinuousTargetOccurence: 2,
	}

	outputRaw, err := stateConf.WaitForStateContext(ctx)
	if out, ok := outputRaw.(*lexmodelsv2.DescribeIntentOutput); ok {
		return out, err
	}

	return nil, err
}

func waitIntentDeleted(ctx context.Context, conn *lexmodelsv2.Client, intentID, botID, botVersion, localeID string, timeout time.Duration) (*lexmodelsv2.DescribeIntentOutput, error) {
	stateConf := &retry.StateChangeConf{
		Pending:    []string{statusNormal},
		Target:     []string{},
		Refresh:    statusIntent(ctx, conn, intentID, botID, botVersion, localeID),
		Timeout:    timeout,
		MinTimeout: 5 * time.Second,
	}

	outputRaw, err := stateConf.WaitForStateContext(ctx)
	if out, ok := outputRaw.(*lexmodelsv2.DescribeIntentOutput); ok {
		return out, err
	}

	return nil, err
}

func statusIntent(ctx context.Context, conn *lexmodelsv2.Client, intentID, botID, botVersion, localeID string) retry.StateRefreshFunc {
	return func() (interface{}, string, error) {
		out, err := findIntentByIDs(ctx, conn, intentID, botID, botVersion, localeID)
		if tfresource.NotFound(err) {
			return nil, "", nil
		}

		if err != nil {
			return nil, "", err
		}

		return out, statusNormal, nil
	}
}

func findIntentByIDs(ctx context.Context, conn *lexmodelsv2.Client, intentID, botID, botVersion, localeID string) (*lexmodelsv2.DescribeIntentOutput, error) {
	in := &lexmodelsv2.DescribeIntentInput{
		BotId:      aws.String(botID),
		BotVersion: aws.String(botVersion),
		IntentId:   aws.String(intentID),
		LocaleId:   aws.String(localeID),
	}

	out, err := conn.DescribeIntent(ctx, in)
	if err != nil {
		var nfe *awstypes.ResourceNotFoundException
		if errors.As(err, &nfe) {
			return nil, &retry.NotFoundError{
				LastError:   err,
				LastRequest: in,
			}
		}

		return nil, err
	}

	if out == nil {
		return nil, tfresource.NewEmptyResultError(in)
	}

	return out, nil
}

type ResourceIntentData struct {
	BotID                  types.String                                                 `tfsdk:"bot_id"`
	BotVersion             types.String                                                 `tfsdk:"bot_version"`
	ClosingSetting         fwtypes.ListNestedObjectValueOf[IntentClosingSetting]        `tfsdk:"closing_setting"`
	ConfirmationSetting    fwtypes.ListNestedObjectValueOf[IntentConfirmationSetting]   `tfsdk:"confirmation_setting"`
	CreationDateTime       fwtypes.Timestamp                                            `tfsdk:"creation_date_time"`
	Description            types.String                                                 `tfsdk:"description"`
	DialogCodeHook         fwtypes.ListNestedObjectValueOf[DialogCodeHookSettings]      `tfsdk:"dialog_code_hook"`
	FulfillmentCodeHook    fwtypes.ListNestedObjectValueOf[FulfillmentCodeHookSettings] `tfsdk:"fulfillment_code_hook"`
	ID                     types.String                                                 `tfsdk:"id"`
	IntentID               types.String                                                 `tfsdk:"intent_id"`
	InitialResponseSetting fwtypes.ListNestedObjectValueOf[InitialResponseSetting]      `tfsdk:"initial_response_setting"`
	InputContext           fwtypes.ListNestedObjectValueOf[InputContext]                `tfsdk:"input_context"`
	KendraConfiguration    fwtypes.ListNestedObjectValueOf[KendraConfiguration]         `tfsdk:"kendra_configuration"`
	LastUpdatedDateTime    fwtypes.Timestamp                                            `tfsdk:"last_updated_date_time"`
	LocaleID               types.String                                                 `tfsdk:"locale_id"`
	Name                   types.String                                                 `tfsdk:"name"`
	OutputContext          fwtypes.ListNestedObjectValueOf[OutputContext]               `tfsdk:"output_context"`
	ParentIntentSignature  types.String                                                 `tfsdk:"parent_intent_signature"`
	SampleUtterance        fwtypes.ListNestedObjectValueOf[SampleUtterance]             `tfsdk:"sample_utterance"`
	SlotPriority           fwtypes.ListNestedObjectValueOf[SlotPriority]                `tfsdk:"slot_priority"`
	Timeouts               timeouts.Value                                               `tfsdk:"timeouts"`
}

type IntentClosingSetting struct {
	Active          types.Bool                                                `tfsdk:"active"`
	ClosingResponse fwtypes.ListNestedObjectValueOf[ResponseSpecification]    `tfsdk:"closing_response"`
	Conditional     fwtypes.ListNestedObjectValueOf[ConditionalSpecification] `tfsdk:"conditional"`
	NextStep        fwtypes.ListNestedObjectValueOf[DialogState]              `tfsdk:"next_step"`
}

type ResponseSpecification struct {
	MessageGroup   fwtypes.ListNestedObjectValueOf[MessageGroup] `tfsdk:"message_group"`
	AllowInterrupt types.Bool                                    `tfsdk:"allow_interrupt"`
}

type MessageGroup struct {
	Message    fwtypes.ListNestedObjectValueOf[Message] `tfsdk:"message"`
	Variations fwtypes.ListNestedObjectValueOf[Message] `tfsdk:"variations"`
}

type Message struct {
	CustomPayload     fwtypes.ListNestedObjectValueOf[CustomPayload]     `tfsdk:"custom_payload"`
	ImageResponseCard fwtypes.ListNestedObjectValueOf[ImageResponseCard] `tfsdk:"image_response_card"`
	PlainTextMessage  fwtypes.ListNestedObjectValueOf[PlainTextMessage]  `tfsdk:"plain_text_message"`
	SSMLMessage       fwtypes.ListNestedObjectValueOf[SSMLMessage]       `tfsdk:"ssml_message"`
}

type CustomPayload struct {
	Value types.String `tfsdk:"value"`
}

type ImageResponseCard struct {
	Title    types.String                            `tfsdk:"title"`
	Button   fwtypes.ListNestedObjectValueOf[Button] `tfsdk:"buttons"`
	ImageURL types.String                            `tfsdk:"image_url"`
	Subtitle types.String                            `tfsdk:"subtitle"`
}

type Button struct {
	Text  types.String `tfsdk:"text"`
	Value types.String `tfsdk:"value"`
}

type PlainTextMessage struct {
	Value types.String `tfsdk:"value"`
}

type SSMLMessage struct {
	Value types.String `tfsdk:"value"`
}

type ConditionalSpecification struct {
	Active            types.Bool                                                `tfsdk:"active"`
	ConditionalBranch fwtypes.ListNestedObjectValueOf[ConditionalBranch]        `tfsdk:"conditional_branch"`
	DefaultBranch     fwtypes.ListNestedObjectValueOf[DefaultConditionalBranch] `tfsdk:"default_branch"`
}

type ConditionalBranch struct {
	Condition fwtypes.ListNestedObjectValueOf[Condition]             `tfsdk:"condition"`
	Name      types.String                                           `tfsdk:"name"`
	NextStep  fwtypes.ListNestedObjectValueOf[DialogState]           `tfsdk:"next_step"`
	Response  fwtypes.ListNestedObjectValueOf[ResponseSpecification] `tfsdk:"response"`
}

type Condition struct {
	ExpressionString types.String `tfsdk:"expression_string"`
}

type DialogState struct {
	DialogAction      fwtypes.ListNestedObjectValueOf[DialogAction]   `tfsdk:"dialog_action"`
	Intent            fwtypes.ListNestedObjectValueOf[IntentOverride] `tfsdk:"intent"`
	SessionAttributes fwtypes.MapValueOf[basetypes.StringValue]       `tfsdk:"session_attributes"`
}

type DialogAction struct {
	Type                fwtypes.StringEnum[awstypes.DialogActionType] `tfsdk:"type"`
	SlotToElicit        types.String                                  `tfsdk:"slot_to_elicit"`
	SuppressNextMessage types.Bool                                    `tfsdk:"suppress_next_message"`
}

type IntentOverride struct {
	Name  types.String                                `tfsdk:"name"`
	Slots fwtypes.ObjectMapValueOf[SlotValueOverride] `tfsdk:"slots"`
}

type SlotValueOverride struct {
	Shape fwtypes.StringEnum[awstypes.SlotShape]     `tfsdk:"shape"`
	Value fwtypes.ListNestedObjectValueOf[SlotValue] `tfsdk:"value"`
	//Values fwtypes.ListNestedObjectValueOf[SlotValueOverride] `tfsdk:"values"`
}

type SlotValue struct {
	InterpretedValue types.String `tfsdk:"interpreted_value"`
}

type DefaultConditionalBranch struct {
	NextStep fwtypes.ListNestedObjectValueOf[DialogState]           `tfsdk:"next_step"`
	Response fwtypes.ListNestedObjectValueOf[ResponseSpecification] `tfsdk:"response"`
}

type IntentConfirmationSetting struct {
	PromptSpecification     fwtypes.ListNestedObjectValueOf[PromptSpecification]                  `tfsdk:"prompt_specification"`
	Active                  types.Bool                                                            `tfsdk:"active"`
	CodeHook                fwtypes.ListNestedObjectValueOf[DialogCodeHookInvocationSetting]      `tfsdk:"code_hook"`
	ConfirmationConditional fwtypes.ListNestedObjectValueOf[ConditionalSpecification]             `tfsdk:"confirmation_conditional"`
	ConfirmationNextStep    fwtypes.ListNestedObjectValueOf[DialogState]                          `tfsdk:"confirmation_next_step"`
	ConfirmationResponse    fwtypes.ListNestedObjectValueOf[ResponseSpecification]                `tfsdk:"confirmation_response"`
	DeclinationConditional  fwtypes.ListNestedObjectValueOf[ConditionalSpecification]             `tfsdk:"declination_conditional"`
	DeclinationNextStep     fwtypes.ListNestedObjectValueOf[DialogState]                          `tfsdk:"declination_next_step"`
	DeclinationResponse     fwtypes.ListNestedObjectValueOf[ResponseSpecification]                `tfsdk:"declination_response"`
	ElicitationCodeHook     fwtypes.ListNestedObjectValueOf[ElicitationCodeHookInvocationSetting] `tfsdk:"elicitation_code_hook"`
	FailureConditional      fwtypes.ListNestedObjectValueOf[ConditionalSpecification]             `tfsdk:"failure_conditional"`
	FailureNextStep         fwtypes.ListNestedObjectValueOf[DialogState]                          `tfsdk:"failure_next_step"`
	FailureResponse         fwtypes.ListNestedObjectValueOf[ResponseSpecification]                `tfsdk:"failure_response"`
}

type PromptSpecification struct {
	MaxRetries                  types.Int64                                           `tfsdk:"max_retries"`
	MessageGroup                fwtypes.ListNestedObjectValueOf[MessageGroup]         `tfsdk:"message_groups"`
	AllowInterrupt              types.Bool                                            `tfsdk:"allow_interrupt"`
	MessageSelectionStrategy    fwtypes.StringEnum[awstypes.MessageSelectionStrategy] `tfsdk:"message_selection_strategy"`
	PromptAttemptsSpecification fwtypes.ObjectMapValueOf[PromptAttemptSpecification]  `tfsdk:"prompt_attempts_specification"`
}

type PromptAttemptSpecification struct {
	AllowedInputTypes              fwtypes.ListNestedObjectValueOf[AllowedInputTypes]              `tfsdk:"allowed_input_types"`
	AllowInterrupt                 types.Bool                                                      `tfsdk:"allow_interrupt"`
	AudioAndDTMFInputSpecification fwtypes.ListNestedObjectValueOf[AudioAndDTMFInputSpecification] `tfsdk:"audio_and_dtmf_input_specification"`
	TextInputSpecification         fwtypes.ListNestedObjectValueOf[TextInputSpecification]         `tfsdk:"text_input_specification"`
}

type AllowedInputTypes struct {
	AllowAudioInput types.Bool `tfsdk:"allow_audio_input"`
	AllowDTMFInput  types.Bool `tfsdk:"allow_dtmf_input"`
}

type AudioAndDTMFInputSpecification struct {
	StartTimeoutMs     types.Int64                                         `tfsdk:"start_timeout_ms"`
	AudioSpecification fwtypes.ListNestedObjectValueOf[AudioSpecification] `tfsdk:"audio_specification"`
	DTMFSpecification  fwtypes.ListNestedObjectValueOf[DTMFSpecification]  `tfsdk:"dtmf_specification"`
}

type AudioSpecification struct {
	EndTimeoutMs types.Int64 `tfsdk:"end_timeout_ms"`
	MaxLengthMs  types.Int64 `tfsdk:"max_length_ms"`
}

type DTMFSpecification struct {
	DeletionCharacter types.String `tfsdk:"deletion_character"`
	EndCharacter      types.String `tfsdk:"end_character"`
	EndTimeoutMs      types.Int64  `tfsdk:"end_timeout_ms"`
	MaxLength         types.Int64  `tfsdk:"max_length"`
}

type TextInputSpecification struct {
	StartTimeoutMs types.Int64 `tfsdk:"start_timeout_ms"`
}

type DialogCodeHookInvocationSetting struct {
	Active                    types.Bool                                             `tfsdk:"active"`
	EnableCodeHookInvocation  types.Bool                                             `tfsdk:"enable_code_hook_invocation"`
	PostCodeHookSpecification fwtypes.ListNestedObjectValueOf[FailureSuccessTimeout] `tfsdk:"post_code_hook_specification"`
	InvocationLabel           types.String                                           `tfsdk:"invocation_label"`
}

type FailureSuccessTimeout struct {
	FailureConditional fwtypes.ListNestedObjectValueOf[ConditionalSpecification] `tfsdk:"failure_conditional"`
	FailureNextStep    fwtypes.ListNestedObjectValueOf[DialogState]              `tfsdk:"failure_next_step"`
	FailureResponse    fwtypes.ListNestedObjectValueOf[ResponseSpecification]    `tfsdk:"failure_response"`
	SuccessConditional fwtypes.ListNestedObjectValueOf[ConditionalSpecification] `tfsdk:"success_conditional"`
	SuccessNextStep    fwtypes.ListNestedObjectValueOf[DialogState]              `tfsdk:"success_next_step"`
	SuccessResponse    fwtypes.ListNestedObjectValueOf[ResponseSpecification]    `tfsdk:"success_response"`
	TimeoutConditional fwtypes.ListNestedObjectValueOf[ConditionalSpecification] `tfsdk:"timeout_conditional"`
	TimeoutNextStep    fwtypes.ListNestedObjectValueOf[DialogState]              `tfsdk:"timeout_next_step"`
	TimeoutResponse    fwtypes.ListNestedObjectValueOf[ResponseSpecification]    `tfsdk:"timeout_response"`
}

type ElicitationCodeHookInvocationSetting struct {
	EnableCodeHookInvocation types.Bool   `tfsdk:"enable_code_hook_invocation"`
	InvocationLabel          types.String `tfsdk:"invocation_label"`
}

type DialogCodeHookSettings struct {
	Enabled types.Bool `tfsdk:"enabled"`
}

type FulfillmentCodeHookSettings struct {
	Enabled                            types.Bool                                                       `tfsdk:"enabled"`
	Active                             types.Bool                                                       `tfsdk:"active"`
	FulfillmentUpdatesSpecification    fwtypes.ListNestedObjectValueOf[FulfillmentUpdatesSpecification] `tfsdk:"fulfillment_updates_specification"`
	PostFulfillmentStatusSpecification fwtypes.ListNestedObjectValueOf[FailureSuccessTimeout]           `tfsdk:"post_fulfillment_status_specification"`
}

type FulfillmentUpdatesSpecification struct {
	Active           types.Bool                                                              `tfsdk:"active"`
	StartResponse    fwtypes.ListNestedObjectValueOf[FulfillmentStartResponseSpecification]  `tfsdk:"start_response"`
	TimeoutInSeconds types.Int64                                                             `tfsdk:"timeout_in_seconds"`
	UpdateResponse   fwtypes.ListNestedObjectValueOf[FulfillmentUpdateResponseSpecification] `tfsdk:"update_response"`
}

type FulfillmentStartResponseSpecification struct {
	DelayInSeconds types.Int64                                   `tfsdk:"delay_in_seconds"`
	MessageGroup   fwtypes.ListNestedObjectValueOf[MessageGroup] `tfsdk:"message_group"`
	AllowInterrupt types.Bool                                    `tfsdk:"allow_interrupt"`
}

type FulfillmentUpdateResponseSpecification struct {
	FrequencyInSeconds types.Int64                                   `tfsdk:"frequency_in_seconds"`
	MessageGroup       fwtypes.ListNestedObjectValueOf[MessageGroup] `tfsdk:"message_group"`
	AllowInterrupt     types.Bool                                    `tfsdk:"allow_interrupt"`
}

type InitialResponseSetting struct {
	CodeHook        fwtypes.ListNestedObjectValueOf[DialogCodeHookInvocationSetting] `tfsdk:"code_hook"`
	Conditional     fwtypes.ListNestedObjectValueOf[ConditionalSpecification]        `tfsdk:"conditional"`
	InitialResponse fwtypes.ListNestedObjectValueOf[ResponseSpecification]           `tfsdk:"initial_response"`
	NextStep        fwtypes.ListNestedObjectValueOf[DialogState]                     `tfsdk:"next_step"`
}

type InputContext struct {
	Name types.String `tfsdk:"name"`
}

type KendraConfiguration struct {
	KendraIndex              types.String `tfsdk:"kendra_index"`
	QueryFilterString        types.String `tfsdk:"query_filter_string"`
	QueryFilterStringEnabled types.Bool   `tfsdk:"query_filter_string_enabled"`
}

type OutputContext struct {
	Name                types.String `tfsdk:"name"`
	TimeToLiveInSeconds types.Int64  `tfsdk:"time_to_live_in_seconds"`
	TurnsToLive         types.Int64  `tfsdk:"turns_to_live"`
}

type SampleUtterance struct {
	Utterance types.String `tfsdk:"utterance"`
}

type SlotPriority struct {
	Priority types.Int64  `tfsdk:"priority"`
	SlotID   types.String `tfsdk:"slot_id"`
}
