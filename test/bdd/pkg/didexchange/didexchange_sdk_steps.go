/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package didexchange

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DATA-DOG/godog"

	"github.com/hyperledger/aries-framework-go/pkg/client/didexchange"
	"github.com/hyperledger/aries-framework-go/pkg/didcomm/common/service"
	diddoc "github.com/hyperledger/aries-framework-go/pkg/doc/did"
	"github.com/hyperledger/aries-framework-go/pkg/framework/didresolver"
	"github.com/hyperledger/aries-framework-go/test/bdd/pkg/context"
)

// DIDExchangeSDKSteps
type DIDExchangeSDKSteps struct {
	bddContext     *context.BDDContext
	actionCh       map[string]chan service.DIDCommAction
	connectionID   map[string]string
	invitations    map[string]*didexchange.Invitation
	postStatesFlag map[string]map[string]chan bool
}

// NewDIDExchangeSDKSteps
func NewDIDExchangeSDKSteps(context *context.BDDContext) *DIDExchangeSDKSteps {
	return &DIDExchangeSDKSteps{
		bddContext:     context,
		actionCh:       make(map[string]chan service.DIDCommAction),
		connectionID:   make(map[string]string),
		invitations:    make(map[string]*didexchange.Invitation),
		postStatesFlag: make(map[string]map[string]chan bool),
	}
}

func (d *DIDExchangeSDKSteps) createInvitation(inviterAgentID string) error {
	invitation, err := d.bddContext.DIDExchangeClients[inviterAgentID].CreateInvitation(inviterAgentID)
	if err != nil {
		return fmt.Errorf("create invitation: %w", err)
	}
	d.invitations[inviterAgentID] = invitation

	invitationBytes, err := json.Marshal(invitation)
	if err != nil {
		return fmt.Errorf("marshal invitation: %w", err)
	}
	logger.Debugf("Agent %s create invitation %s", inviterAgentID, invitationBytes)

	return nil
}

func (d *DIDExchangeSDKSteps) createInvitationWithDID(inviterAgentID string) error {
	invitation, err := d.bddContext.DIDExchangeClients[inviterAgentID].CreateInvitationWithDID(inviterAgentID, d.bddContext.PublicDIDs[inviterAgentID].ID)
	if err != nil {
		return fmt.Errorf("failed to create invitation: %w", err)
	}
	d.invitations[inviterAgentID] = invitation
	invitationBytes, err := json.Marshal(invitation)
	if err != nil {
		return fmt.Errorf("failed to marshal invitation: %w", err)
	}

	logger.Debugf("Agent %s create invitation %s", inviterAgentID, invitationBytes)
	return nil
}

func (d *DIDExchangeSDKSteps) waitForPublicDID(agentID string, maxSeconds int) error {
	_, err := resolveDID(d.bddContext.AgentCtx[agentID].DIDResolver(), d.bddContext.PublicDIDs[agentID].ID, maxSeconds)
	return err
}

func (d *DIDExchangeSDKSteps) receiveInvitation(inviteeAgentID, inviterAgentID string) error {
	connectionID, err := d.bddContext.DIDExchangeClients[inviteeAgentID].HandleInvitation(d.invitations[inviterAgentID])
	if err != nil {
		return fmt.Errorf("failed to handle invitation: %w", err)
	}

	d.connectionID[inviteeAgentID] = connectionID

	return nil
}

func (d *DIDExchangeSDKSteps) waitForPostEvent(agentID, statesValue string) error {
	states := strings.Split(statesValue, ",")
	for _, state := range states {
		select {
		case <-d.postStatesFlag[agentID][state]:
		case <-time.After(5 * time.Second):
			return fmt.Errorf("timeout waiting for post state event %s", state)
		}
	}
	return nil
}

func (d *DIDExchangeSDKSteps) validateConnection(agentID, stateValue string) error {
	conn, err := d.bddContext.DIDExchangeClients[agentID].GetConnection(d.connectionID[agentID])
	if err != nil {
		return fmt.Errorf("failed to query connection by id: %w", err)
	}
	if conn.State != stateValue {
		return fmt.Errorf("state from connection %s not equal %s", conn.State, stateValue)
	}
	return nil
}

func (d *DIDExchangeSDKSteps) approveRequest(agentID string) error {
	go func() {
		for e := range d.actionCh[agentID] {
			switch v := e.Properties.(type) {
			case didexchange.Event:
				d.connectionID[agentID] = v.ConnectionID()
			case error:
				panic(fmt.Sprintf("Service processing failed: %s", v))
			}

			clientOpts := d.getClientOptions(agentID)
			e.Continue(clientOpts)
		}
	}()

	return nil
}

func (d *DIDExchangeSDKSteps) getClientOptions(agentID string) interface{} {
	var clientOpts interface{}
	pubDID, ok := d.bddContext.PublicDIDs[agentID]
	if ok {
		clientOpts = &clientOptions{publicDID: pubDID.ID}
		logger.Debugf("Agent %s will use public DID %s:", agentID, pubDID.ID)
	}

	return clientOpts
}

type clientOptions struct {
	publicDID string
}

func (copts *clientOptions) PublicDID() string {
	return copts.publicDID
}

func (d *DIDExchangeSDKSteps) createDIDExchangeClient(agentID string) error {
	// create new did exchange client
	didexchangeClient, err := didexchange.New(d.bddContext.AgentCtx[agentID])
	if err != nil {
		return fmt.Errorf("failed to create new didexchange client: %w", err)
	}

	actionCh := make(chan service.DIDCommAction, 1)
	if err = didexchangeClient.RegisterActionEvent(actionCh); err != nil {
		return fmt.Errorf("failed to register action event: %w", err)
	}
	d.actionCh[agentID] = actionCh

	d.bddContext.DIDExchangeClients[agentID] = didexchangeClient
	return nil
}

func (d *DIDExchangeSDKSteps) registerPostMsgEvent(agentID, statesValue string) error {
	statusCh := make(chan service.StateMsg, 1)
	if err := d.bddContext.DIDExchangeClients[agentID].RegisterMsgEvent(statusCh); err != nil {
		return fmt.Errorf("failed to register msg event: %w", err)
	}
	states := strings.Split(statesValue, ",")
	d.initializeStates(agentID, states)

	go d.eventListener(statusCh, agentID, states)

	return nil
}

func (d *DIDExchangeSDKSteps) initializeStates(agentID string, states []string) {
	d.postStatesFlag[agentID] = make(map[string]chan bool)
	for _, state := range states {
		d.postStatesFlag[agentID][state] = make(chan bool)
	}
}

func (d *DIDExchangeSDKSteps) eventListener(statusCh chan service.StateMsg, agentID string, states []string) {
	for e := range statusCh {
		switch v := e.Properties.(type) {
		case error:
			panic(fmt.Sprintf("Service processing failed: %s", v))
		}

		if e.Type == service.PostState {
			dst := &bytes.Buffer{}
			if err := json.Indent(dst, e.Msg.Payload, "", "  "); err != nil {
				panic(err)
			}
			if e.StateID != "invited" {
				logger.Debugf("Agent %s done processing %s message \n%s\n*****", agentID, e.Msg.Header.Type, dst)
			}
			for _, state := range states {
				// receive the events
				if e.StateID == state {
					d.postStatesFlag[agentID][state] <- true
				}

			}
		}
	}
}

func resolveDID(resolver didresolver.Resolver, did string, maxRetry int) (*diddoc.Doc, error) {
	var err error
	var doc *diddoc.Doc
	for i := 1; i <= maxRetry; i++ {
		doc, err = resolver.Resolve(did)
		if err == nil || !strings.Contains(err.Error(), "DID does not exist") {
			return doc, err
		}

		time.Sleep(1 * time.Second)
		logger.Debugf("Waiting for public did to be published in sidtree: %d second(s)\n", i)
	}

	return doc, err
}

// RegisterSteps registers did exchange steps
func (d *DIDExchangeSDKSteps) RegisterSteps(s *godog.Suite) {
	s.Step(`^"([^"]*)" creates invitation$`, d.createInvitation)
	s.Step(`^"([^"]*)" creates invitation with public DID$`, d.createInvitationWithDID)
	s.Step(`^"([^"]*)" waits for public did to become available in sidetree for up to (\d+) seconds$`, d.waitForPublicDID)
	s.Step(`^"([^"]*)" receives invitation from "([^"]*)"$`, d.receiveInvitation)
	s.Step(`^"([^"]*)" waits for post state event "([^"]*)"$`, d.waitForPostEvent)
	s.Step(`^"([^"]*)" retrieves connection record and validates that connection state is "([^"]*)"$`, d.validateConnection)
	s.Step(`^"([^"]*)" creates did exchange client$`, d.createDIDExchangeClient)
	s.Step(`^"([^"]*)" approves did exchange request`, d.approveRequest)
	s.Step(`^"([^"]*)" approves invitation request`, d.approveRequest)
	s.Step(`^"([^"]*)" registers to receive notification for post state event "([^"]*)"$`, d.registerPostMsgEvent)
}