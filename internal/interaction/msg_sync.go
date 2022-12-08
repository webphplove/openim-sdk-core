package interaction

import (
	"github.com/golang/protobuf/proto"
	"open_im_sdk/internal/full"
	"open_im_sdk/pkg/common"
	"open_im_sdk/pkg/constant"
	"open_im_sdk/pkg/db/db_interface"
	"open_im_sdk/pkg/log"
	"open_im_sdk/pkg/server_api_params"
	"open_im_sdk/pkg/utils"
	"open_im_sdk/sdk_struct"
)

type SeqPair struct {
	BeginSeq uint32
	EndSeq   uint32
}

type MsgSync struct {
	full *full.Full
	db_interface.DataBase
	*Ws
	LoginUserID        string
	conversationCh     chan common.Cmd2Value
	PushMsgAndMaxSeqCh chan common.Cmd2Value

	selfMsgSync *SelfMsgSync
	//selfMsgSyncLatestModel *SelfMsgSyncLatestModel
	//superGroupMsgSync *SuperGroupMsgSync
	isSyncFinished            bool
	readDiffusionGroupMsgSync *ReadDiffusionGroupMsgSync
}

func (m *MsgSync) compareSeq() {
	operationID := utils.OperationIDGenerator()
	m.selfMsgSync.compareSeq(operationID)
	m.readDiffusionGroupMsgSync.compareSeq(operationID)
}

func (m *MsgSync) doConnectFailed(cmd common.Cmd2Value) {
	operationID := utils.OperationIDGenerator()
	log.Warn(operationID, " MsgSyncBegin ")
	m.readDiffusionGroupMsgSync.TriggerCmdNewMsgCome(nil, operationID, constant.MsgSyncBegin)
	m.readDiffusionGroupMsgSync.TriggerCmdNewMsgCome(nil, operationID, constant.MsgSyncFailed)
	log.Warn(operationID, " MsgSyncFailed ")
}

func (m *MsgSync) doConnectSuccess(cmd common.Cmd2Value) {
	operationID := utils.OperationIDGenerator()
	log.Warn(operationID, " MsgSyncBegin ")
	m.readDiffusionGroupMsgSync.TriggerCmdNewMsgCome(nil, operationID, constant.MsgSyncBegin)
	seqInfo := m.getMaxSeq()
	if seqInfo != nil {
		log.Warn(operationID, "seqInfo ", seqInfo.String())
		if err := m.selfMsgSync.DoConnectSuccess(seqInfo.MinSeq, seqInfo.MaxSeq, operationID); err != nil {
			log.Error(operationID, "DoConnectSuccess failed MsgSyncFailed CloseConn", err.Error())
			m.readDiffusionGroupMsgSync.TriggerCmdNewMsgCome(nil, operationID, constant.MsgSyncFailed)
			m.CloseConn(operationID)
			return
		}
		if err := m.readDiffusionGroupMsgSync.DoConnectSuccess(seqInfo.GroupMaxAndMinSeq, operationID); err != nil {
			log.Error(operationID, "DoConnectSuccess failed MsgSyncFailed CloseConn", err.Error())
			m.readDiffusionGroupMsgSync.TriggerCmdNewMsgCome(nil, operationID, constant.MsgSyncFailed)
			m.CloseConn(operationID)
			return
		}
		log.Warn(operationID, "DoConnectSuccess MsgSyncEnd")
		m.readDiffusionGroupMsgSync.TriggerCmdNewMsgCome(nil, operationID, constant.MsgSyncEnd)
	} else {
		log.Error(operationID, "getMaxSeq() == nil, MsgSyncFailed")
		m.readDiffusionGroupMsgSync.TriggerCmdNewMsgCome(nil, operationID, constant.MsgSyncFailed)
	}
}

func (m *MsgSync) getMaxSeq() *server_api_params.GetMaxAndMinSeqResp {
	operationID := utils.OperationIDGenerator()
	var groupIDList []string
	var err error
	groupIDList, err = m.full.GetReadDiffusionGroupIDList(operationID)
	if err != nil {
		log.Error(operationID, "GetReadDiffusionGroupIDList failed ", err.Error())
	}
	log.Debug(operationID, "get GetJoinedSuperGroupIDList ", groupIDList)
	resp, err := m.SendReqWaitResp(&server_api_params.GetMaxAndMinSeqReq{UserID: m.LoginUserID, GroupIDList: groupIDList}, constant.WSGetNewestSeq, 30, 0, m.LoginUserID, operationID)
	if err != nil {
		log.Error(operationID, "SendReqWaitResp failed ", err.Error(), constant.WSGetNewestSeq, m.LoginUserID)
		return nil
	}

	var wsSeqResp server_api_params.GetMaxAndMinSeqResp
	err = proto.Unmarshal(resp.Data, &wsSeqResp)
	if err != nil {
		log.Error(operationID, "Unmarshal failed, close conn", err.Error())
		return nil
	}
	return &wsSeqResp
}

func (m *MsgSync) doMaxSeq(cmd common.Cmd2Value) {
	m.readDiffusionGroupMsgSync.doMaxSeq(cmd)
	m.selfMsgSync.doMaxSeq(cmd)
}

func (m *MsgSync) doPushMsg(cmd common.Cmd2Value) {
	msg := cmd.Value.(sdk_struct.CmdPushMsgToMsgSync).Msg
	switch msg.SessionType {
	case constant.SuperGroupChatType:
		m.readDiffusionGroupMsgSync.doPushMsg(cmd)
	default:
		m.selfMsgSync.doPushMsg(cmd)
	}
}

func (m *MsgSync) Work(cmd common.Cmd2Value) {
	switch cmd.Cmd {
	case constant.CmdPushMsg:
		m.doPushMsg(cmd)
	case constant.CmdMaxSeq:
		m.doMaxSeq(cmd)
	case constant.CmdConnectFailed:
		m.doConnectFailed(cmd)
	case constant.CmdConnectSuccess:
		m.doConnectSuccess(cmd)
	default:
		log.Error("", "cmd failed ", cmd.Cmd)
	}
}

func (m *MsgSync) GetCh() chan common.Cmd2Value {
	return m.PushMsgAndMaxSeqCh
}

func NewMsgSync(dataBase db_interface.DataBase, ws *Ws, loginUserID string, ch chan common.Cmd2Value, pushMsgAndMaxSeqCh chan common.Cmd2Value,
	joinedSuperGroupCh chan common.Cmd2Value, full *full.Full) *MsgSync {
	p := &MsgSync{DataBase: dataBase, Ws: ws, LoginUserID: loginUserID, conversationCh: ch, PushMsgAndMaxSeqCh: pushMsgAndMaxSeqCh, full: full}
	//	p.superGroupMsgSync = NewSuperGroupMsgSync(dataBase, ws, loginUserID, ch, joinedSuperGroupCh)
	p.selfMsgSync = NewSelfMsgSync(dataBase, ws, loginUserID, ch)
	p.readDiffusionGroupMsgSync = NewReadDiffusionGroupMsgSync(dataBase, ws, loginUserID, ch, joinedSuperGroupCh)
	//	p.selfMsgSync = NewSelfMsgSyncLatestModel(dataBase, ws, loginUserID, ch)
	p.compareSeq()
	go common.DoListener(p)
	return p
}
