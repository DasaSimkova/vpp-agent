*** Settings ***
Library      OperatingSystem
#Library      RequestsLibrary
#Library      SSHLibrary      timeout=60s
#Library      String

Resource     ../../../variables/${VARIABLES}_variables.robot
Resource     ../../../libraries/all_libs.robot
Resource    ../../../libraries/pretty_keywords.robot

Suite Setup       Testsuite Setup
#Suite Teardown    Testsuite Teardown
Test Setup        TestSetup
Test Teardown     TestTeardown

*** Variables ***
${VARIABLES}=        common
${ENV}=              common
${NAME_VPP1_TAP0}=         vpp1_tap0
${NAME_VPP2_TAP0}=         vpp2_tap0
${NAME_LINUX_VPP1_TAP0}=       linux_${NAME_VPP1_TAP0}
${NAME_LINUX_VPP2_TAP0}=       linux_${NAME_VPP2_TAP0}
${MAC_VPP1_TAP0}=         12:21:21:11:11:11
${MAC_VPP2_TAP0}=         32:21:21:11:11:11
${VPP1_MEMIF0_MAC}=           1a:00:00:11:11:11
${VPP2_MEMIF0_MAC}=           2a:00:00:22:22:22
${VPP1_IP}=               10.10.1.0
${VPP2_IP}=               10.10.2.0
${IP_VPP1_TAP0}=          10.10.1.1
${IP_VPP2_TAP0}=          10.10.2.1
${IP_LINUX_VPP1_TAP0}=    10.10.1.2
${IP_LINUX_VPP2_TAP0}=    10.10.2.2

${PREFIX}=           24
${MTU}=              4800
${ENABLED}=          true
${CONFIG_SLEEP}=       1s
${RESYNC_SLEEP}=       1s
# wait for resync vpps after restart
${RESYNC_WAIT}=        30s

*** Test Cases ***
Configure Environment
    [Tags]    setup
    Configure Environment 1

#Show Interfaces Before Setup
#    ${out}=    vpp_term: Show Interfaces    agent_vpp_1
#    ${out}=    vpp_term: Show Interfaces    agent_vpp_2
#
#Setup TAP Interface On VPP1 And Linux
#    vpp_ctl: Put TAP Interface With IP    node=agent_vpp_1    name=${NAME_VPP1_TAP0}    mac=${MAC_VPP1_TAP0}    ip=${IP_VPP1_TAP0}    host_if_name=${NAME_LINUX_VPP1_TAP0}
#    linux: Set Host TAP Interface    node=agent_vpp_1    name=${NAME_LINUX_VPP1_TAP0}    ip=${IP_LINUX_VPP1_TAP0}    prefix=${PREFIX}    node_network_ip=${VPP1_IP}
#
#Check Interface TAP Presence On VPP1
#    ${actual_state}=    vpp_term: Check TAP interface State    agent_vpp_1    ${NAME_VPP1_TAP0}    mac=${MAC_VPP1_TAP0}    ipv4=${IP_VPP1_TAP0}/${PREFIX}    state=up
#
#Check Linux Interface TAP1 Presence On VPP1
#    ${ints}=    linux: Get Linux Interfaces    agent_vpp_1
#    ${actual_state}=     Pick Linux Interface    ${ints}    ${NAME_LINUX_VPP1_TAP0}
#    Log List             ${actual_state}
#    Should Contain    ${actual_state}    ipv4=${IP_LINUX_VPP1_TAP0}/${PREFIX}
#
#Check Ping From VPP1 To Linux TAP Interface
#    vpp_term: Check Ping    agent_vpp_1    ${IP_LINUX_VPP1_TAP0}
#
#Add VPP1_memif0 Interface
#    vpp_term: Interface Not Exists    node=agent_vpp_1    mac=${VPP1_MEMIF0_MAC}
##    Create Master vpp1_memif0 on agent_vpp_1 With IP 192.168.1.1, MAC ${VPP1_MEMIF0_MAC}, key 1 and m0.sock socket
##    ${state}=    vpp_ctl: Get VPP Interface State As Json    agent_vpp_1    vpp1_memif0
#
#    Create loopback interface bvi_loop0 on agent_vpp_1 with ip 10.10.3.1/24 and mac 8a:f1:be:90:00:00
#    Create Master memif0 on agent_vpp_1 with MAC 02:f1:be:90:00:00, key 1 and m0.sock socket
#    Create bridge domain bd1 With Autolearn on agent_vpp_1 with interfaces bvi_loop0, memif0
#    ${out}=    vpp_term: Show Interfaces    agent_vpp_1
#    Log    ${out}
#
#Setup TAP Interface On VPP2 And Linux
#    vpp_ctl: Put TAP Interface With IP    node=agent_vpp_2    name=${NAME_VPP2_TAP0}    mac=${MAC_VPP2_TAP0}    ip=${IP_VPP2_TAP0}    host_if_name=${NAME_LINUX_VPP2_TAP0}
#    linux: Set Host TAP Interface    node=agent_vpp_2    name=${NAME_LINUX_VPP2_TAP0}    ip=${IP_LINUX_VPP2_TAP0}    prefix=${PREFIX}    node_network_ip=${VPP2_IP}
#
#Check Interface TAP1 Presence On VPP2
#    ${actual_state}=    vpp_term: Check TAP interface State    agent_vpp_2    ${NAME_VPP2_TAP0}    mac=${MAC_VPP2_TAP0}    ipv4=${IP_VPP2_TAP0}/${PREFIX}    state=up
#
#Check Linux Interface TAP Presence On VPP2
#    ${ints}=    linux: Get Linux Interfaces    agent_vpp_2
#    ${actual_state}=     Pick Linux Interface    ${ints}    ${NAME_LINUX_VPP2_TAP0}
#    Log List             ${actual_state}
#    Should Contain    ${actual_state}    ipv4=${IP_LINUX_VPP2_TAP0}/${PREFIX}
#
#Check Ping From VPP2 To Host TAP1 Interface
#    vpp_term: Check Ping    agent_vpp_2    ${IP_LINUX_VPP2_TAP0}
#
#Add VPP2_memif0 Interface
#    vpp_term: Interface Not Exists    node=agent_vpp_2    mac=${VPP2_MEMIF0_MAC}
##    Create Slave vpp2_memif0 on agent_vpp_2 With IP 192.168.1.2, MAC ${VPP2_MEMIF0_MAC}, key 1 and m0.sock socket
##    ${state}=    vpp_ctl: Get VPP Interface State As Json    agent_vpp_2    vpp2_memif0
#    Create loopback interface bvi_loop0 on agent_vpp_2 with ip 10.10.3.2/24 and mac 8a:f1:be:90:00:00
#    Create Master memif0 on agent_vpp_1 with MAC 02:f1:be:90:00:00, key 1 and m0.sock socket
#    Create bridge domain bd1 With Autolearn on agent_vpp_2 with interfaces bvi_loop0, memif0
#    ${out}=    vpp_term: Show Interfaces    agent_vpp_2
#    Log    ${out}
#
#Create Routes
#    Create Route On agent_vpp_1 With IP 10.10.2.0/24 With Next Hop 10.10.3.1 And Vrf Id 0
#    Create Route On agent_vpp_2 With IP 10.10.1.0/24 With Next Hop 10.10.3.2 And Vrf Id 0
#
#Check Ping From VPP1 to VPP2
#    linux: Check Ping    agent_vpp_1    10.10.3.2
#
#Check Ping From VPP2 to VPP1
#    linux: Check Ping    agent_vpp_2    10.10.3.1

#    Ping From agent_vpp_1 To 10.1.1.2
#    Ping From agent_vpp_1 To 20.1.1.2
#    Ping From agent_vpp_2 To 20.1.1.2
#    Ping From agent_vpp_3 To 10.1.1.2
#
##Create Route From VPP1 To VPP2
#    Create Route On agent_vpp_1 With IP 20.20.1.0/24 With Next Hop 192.168.1.1 And Vrf Id 0
#
#Create Route From VPP2 To VPP1
#    Create Route On agent_vpp_2 With IP 10.10.1.0/24 With Next Hop 192.168.1.2 And Vrf Id 0
#
##    ${key1}=    Set Variable    /vnf-agent/agent_vpp_1/vpp/config/v1/vrf/0
##    ${out}=    vpp_ctl: Read Key    ${key1}    vpp_agent_ctl
##    ${key2}=    Set Variable    /vnf-agent/agent_vpp_2/vpp/config/v1/vrf/0
##    ${out}=    vpp_ctl: Read Key    ${key2}    vpp_agent_ctl
##    ${out}=    Execute In Container    agent_vpp_1    ip route list
##    ${out}=    Execute In Container    agent_vpp_2    ip route list
#
#Check Ping From VPP1 to VPP2
#    linux: Check Ping    agent_vpp_1    20.20.1.1
#
#Check Ping From VPP2 to VPP1
#    linux: Check Ping    agent_vpp_2    10.10.1.1
#
#Sleep For Debugging
#    Sleep    30 minutes
#
#Delete memif Interfaces
#    vpp_ctl: Delete VPP Interface    node=agent_vpp_1    name=vpp1_memif1
#    vpp_ctl: Delete VPP Interface    node=agent_vpp_2    name=vpp2_memif1

*** Keywords ***
TestSetup
    Make Datastore Snapshots    ${TEST_NAME}_test_setup

TestTeardown
    Make Datastore Snapshots    ${TEST_NAME}_test_teardown

