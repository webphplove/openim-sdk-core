package rtc

import (
	"errors"
	ws "open_im_sdk/internal/interaction"
	"open_im_sdk/open_im_sdk_callback"
	"open_im_sdk/pkg/common"
	"open_im_sdk/pkg/log"
	"open_im_sdk/pkg/sdk_params_callback"
	api "open_im_sdk/pkg/server_api_params"
	"open_im_sdk/pkg/utils"

	"strings"
)

type LiveSignaling struct {
	*ws.Ws
	listener    open_im_sdk_callback.OnSignalingListener
	loginUserID string
}

func (s *LiveSignaling) invite(req *api.SignalInviteReq, callback open_im_sdk_callback.Base, operationID string) sdk_params_callback.InviteCallback {
	var signalReq api.SignalReq
	*signalReq.GetInvite() = *req
	resp, err := s.SendSignalingReqWaitResp(&signalReq, operationID)
	common.CheckAnyErrCallback(callback, 3001, err, operationID)
	switch payload := resp.Payload.(type) {
	case *api.SignalResp_Invite:
		s.waitPush(req, operationID)
		return sdk_params_callback.InviteCallback(payload.Invite)
	default:
		log.Error(operationID, "resp payload type failed ", payload)
		common.CheckAnyErrCallback(callback, 3002, errors.New("resp payload type failed"), operationID)
		return nil
	}
}

func (s *LiveSignaling) waitPush(req *api.SignalReq, operationID string) {
	var invt api.InvitationInfo
	switch payload := req.Payload.(type) {
	case *api.SignalReq_Invite:
		invt = *payload.Invite.Invitation
	case *api.SignalReq_InviteInGroup:
		invt = *payload.InviteInGroup.Invitation
	}

	for _, v := range invt.InviteeUserIDList {
		go func() {
			push, err := s.SignalingWaitPush(invt.InviterUserID, v, invt.RoomID, invt.Timeout, operationID)
			if err != nil {
				if strings.Contains(err.Error(), "timeout") {
					log.Error(operationID, "wait push timeout ", err.Error(), invt.InviterUserID, v, invt.RoomID, invt.Timeout)

				} else {
					log.Error(operationID, "other failed ", err.Error(), invt.InviterUserID, v, invt.RoomID, invt.Timeout)
				}
				return
			}
			s.doSignalPush(push)
		}()
	}
}

func (s *LiveSignaling) doSignalPush(req *api.SignalReq) {
	//payload.Accept
	switch payload := req.Payload.(type) {
	case *api.SignalReq_Invite:
		s.listener.OnReceiveNewInvitation(utils.StructToJsonString(payload.Invite))
	case *api.SignalReq_Accept:
		s.listener.OnInviteeAccepted(utils.StructToJsonString(payload.Accept))
	case *api.SignalReq_Reject:
		s.listener.OnInviteeRejected(utils.StructToJsonString(payload.Reject))
	case *api.SignalReq_Cancel:
		s.listener.OnInvitationCancelled(utils.StructToJsonString(payload.Cancel))
	default:
		log.Error("", "payload type failed ")
	}
}

func (s *LiveSignaling) inviteInGroup(groupID string, inviteeUserIDList []string, customData string, offlinePushInfo *api.OfflinePushInfo, timeout uint32, callback open_im_sdk_callback.Base, operationID string) sdk_params_callback.InviteInGroupCallback {
	return nil
}

func (s *LiveSignaling) SetListener(listener open_im_sdk_callback.OnSignalingListener, operationID string) {
	s.listener = listener
}

func (s *LiveSignaling) handleSignaling(req *api.SignalReq, callback open_im_sdk_callback.Base, operationID string) {
	resp, err := s.SendSignalingReqWaitResp(req, operationID)
	if err != nil {
		log.NewError(operationID, utils.GetSelfFuncName(), "SendSignalingReqWaitResp error", err.Error())
		//callback.OnError()
	}
	common.CheckAnyErrCallback(callback, 3001, err, operationID)
	switch payload := resp.Payload.(type) {
	case *api.SignalResp_Accept:
		callback.OnSuccess(utils.StructToJsonString(sdk_params_callback.AcceptCallback(payload.Accept)))
	case *api.SignalResp_Reject:
		callback.OnSuccess(utils.StructToJsonString(sdk_params_callback.RejectCallback(payload.Reject)))
	case *api.SignalResp_HungUp:
		callback.OnSuccess(utils.StructToJsonString(sdk_params_callback.HungUpCallback(payload.HungUp)))
	case *api.SignalResp_Cancel:
		callback.OnSuccess(utils.StructToJsonString(sdk_params_callback.CancelCallback(payload.Cancel)))
	case *api.SignalResp_Invite:
		callback.OnSuccess(utils.StructToJsonString(sdk_params_callback.InviteCallback(payload.Invite)))
	default:
		log.Error(operationID, "resp payload type failed ", payload)
		common.CheckAnyErrCallback(callback, 3002, errors.New("resp payload type failed"), operationID)
	}
	switch payload := req.Payload.(type) {
	case *api.SignalReq_Invite:
		s.waitPush(payload.Invite, operationID)
	case *api.SignalReq_InviteInGroup:
		s.waitPush(req, operationID)
	}
}