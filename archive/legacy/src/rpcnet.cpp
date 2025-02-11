// Copyright (c) 2009-2012 Bitcoin Developers
// Copyright (c) 2012-2013 PPCoin developers
// Copyright (c) 2013 Primecoin developers
// Distributed under the MIT/X11 software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

#include "net.h"
#include "bitcoinrpc.h"
#include "alert.h"
#include "base58.h"

using namespace json_spirit;
using namespace std;

Value getconnectioncount(const Array& params, bool fHelp)
{
    if (fHelp || params.size() != 0)
        throw runtime_error(
            "getconnectioncount\n"
            "Returns the number of connections to other nodes.");

    LOCK(cs_vNodes);
    return (int)vNodes.size();
}

static void CopyNodeStats(std::vector<CNodeStats>& vstats)
{
    vstats.clear();

    LOCK(cs_vNodes);
    vstats.reserve(vNodes.size());
    BOOST_FOREACH(CNode* pnode, vNodes) {
        CNodeStats stats;
        pnode->copyStats(stats);
        vstats.push_back(stats);
    }
}

Value getpeerinf.Ln(const Array& params, bool fHelp)
{
    if (fHelp || params.size() != 0)
        throw runtime_error(
            "getpeerinfo\n"
            "Returns data about each connected network node.");

    vector<CNodeStats> vstats;
    CopyNodeStats(vstats);

    Array ret;

    BOOST_FOREACH(const CNodeStats& stats, vstats) {
        Object obj;

        obj.push_back(Pair("addr", stats.addrName));
        obj.push_back(Pair("services", strprintf("%08" PRI64x, stats.nServices)));
        obj.push_back(Pair("lastsend", (boost::int64_t)stats.nLastSend));
        obj.push_back(Pair("lastrecv", (boost::int64_t)stats.nLastRecv));
        obj.push_back(Pair("bytessent", (boost::int64_t)stats.nSendBytes));
        obj.push_back(Pair("bytesrecv", (boost::int64_t)stats.nRecvBytes));
        obj.push_back(Pair("conntime", (boost::int64_t)stats.nTimeConnected));
        obj.push_back(Pair("version", stats.nVersion));
        obj.push_back(Pair("subver", stats.strSubVer));
        obj.push_back(Pair("inbound", stats.fInbound));
        obj.push_back(Pair("startingheight", stats.nStartingHeight));
        obj.push_back(Pair("banscore", stats.nMisbehavior));
        if (stats.fSyncNode)
            obj.push_back(Pair("syncnode", true));

        ret.push_back(obj);
    }

    return ret;
}

Value addnode(const Array& params, bool fHelp)
{
    string strCommand;
    if (params.size() == 2)
        strCommand = params[1].get_str();
    if (fHelp || params.size() != 2 ||
        (strCommand != "onetry" && strCommand != "add" && strCommand != "remove"))
        throw runtime_error(
            "addnode <node> <add|remove|onetry>\n"
            "Attempts add or remove <node> from the addnode list or try a connection to <node> once.");

    string strNode = params[0].get_str();

    if (strCommand == "onetry")
    {
        CAddress addr;
        ConnectNode(addr, strNode.c_str());
        return Value::null;
    }

    LOCK(cs_vAddedNodes);
    vector<string>::iterator it = vAddedNodes.begin();
    for(; it != vAddedNodes.end(); it++)
        if (strNode == *it)
            break;

    if (strCommand == "add")
    {
        if (it != vAddedNodes.end())
            throw JSONRPCError(-23, "Error: Node already added");
        vAddedNodes.push_back(strNode);
    }
    else if(strCommand == "remove")
    {
        if (it == vAddedNodes.end())
            throw JSONRPCError(-24, "Error: Node has not been added.");
        vAddedNodes.erase(it);
    }

    return Value::null;
}

Value getaddednodeinf.Ln(const Array& params, bool fHelp)
{
    if (fHelp || params.size() < 1 || params.size() > 2)
        throw runtime_error(
            "getaddednodeinfo <dns> [node]\n"
            "Returns information about the given added node, or all added nodes\n"
            "(note that onetry addnodes are not listed here)\n"
            "If dns is false, only a list of added nodes will be provided,\n"
            "otherwise connected information will also be available.");

    bool fDns = params[0].get_bool();

    list<string> laddedNodes(0);
    if (params.size() == 1)
    {
        LOCK(cs_vAddedNodes);
        BOOST_FOREACH(string& strAddNode, vAddedNodes)
            laddedNodes.push_back(strAddNode);
    }
    else
    {
        string strNode = params[1].get_str();
        LOCK(cs_vAddedNodes);
        BOOST_FOREACH(string& strAddNode, vAddedNodes)
            if (strAddNode == strNode)
            {
                laddedNodes.push_back(strAddNode);
                break;
            }
        if (laddedNodes.size() == 0)
            throw JSONRPCError(-24, "Error: Node has not been added.");
    }

    if (!fDns)
    {
        Object ret;
        BOOST_FOREACH(string& strAddNode, laddedNodes)
            ret.push_back(Pair("addednode", strAddNode));
        return ret;
    }

    Array ret;

    list<pair<string, vector<CService> > > laddedAddreses(0);
    BOOST_FOREACH(string& strAddNode, laddedNodes)
    {
        vector<CService> vservNode(0);
        if(Lookup(strAddNode.c_str(), vservNode, Params().GetDefaultPort(), fNameLookup, 0))
            laddedAddreses.push_back(make_pair(strAddNode, vservNode));
        else
        {
            Object obj;
            obj.push_back(Pair("addednode", strAddNode));
            obj.push_back(Pair("connected", false));
            Array addresses;
            obj.push_back(Pair("addresses", addresses));
        }
    }

    LOCK(cs_vNodes);
    for (list<pair<string, vector<CService> > >::iterator it = laddedAddreses.begin(); it != laddedAddreses.end(); it++)
    {
        Object obj;
        obj.push_back(Pair("addednode", it->first));

        Array addresses;
        bool fConnected = false;
        BOOST_FOREACH(CService& addrNode, it->second)
        {
            bool fFound = false;
            Object node;
            node.push_back(Pair("address", addrNode.ToString()));
            BOOST_FOREACH(CNode* pnode, vNodes)
                if (pnode->addr == addrNode)
                {
                    fFound = true;
                    fConnected = true;
                    node.push_back(Pair("connected", pnode->fInbound ? "inbound" : "outbound"));
                    break;
                }
            if (!fFound)
                node.push_back(Pair("connected", "false"));
            addresses.push_back(node);
        }
        obj.push_back(Pair("connected", fConnected));
        obj.push_back(Pair("addresses", addresses));
        ret.push_back(obj);
    }

    return ret;
}

Value sendalert(const Array& params, bool fHelp)
 {
     if (fHelp || params.size() < 6)
         throw runtime_error(
             "sendalert <message> <privatekey> <minver> <maxver> <priority> <id> [cancelupto]\n"
             "<message> is the alert text message\n"
             "<privatekey> is base58 hex string of alert master private key\n"
             "<minver> is the minimum applicable internal client version\n"
             "<maxver> is the maximum applicable internal client version\n"
             "<priority> is integer priority number\n"
             "<id> is the alert id\n"
             "[cancelupto] cancels all alert id's up to this number\n"
             "Returns true or false.");
 
     // Prepare the alert message
     CAlert alert;
     alert.strStatusBar = params[0].get_str();
     alert.nMinVer = params[2].get_int();
     alert.nMaxVer = params[3].get_int();
     alert.nPriority = params[4].get_int();
     alert.nID = params[5].get_int();
     if (params.size() > 6)
         alert.nCancel = params[6].get_int();
     alert.nVersion = PROTOCOL_VERSION;
     alert.nRelayUntil = GetAdjustedTime() + 365*24*60*60;
     alert.nExpiration = GetAdjustedTime() + 365*24*60*60;
 
     CDataStream sMsg(SER_NETWORK, PROTOCOL_VERSION);
     sMsg << (CUnsignedAlert)alert;
     alert.vchMsg = vector<unsigned char>(sMsg.begin(), sMsg.end());
 
     // Prepare master key and sign alert message
     CBitcoinSecret vchSecret;
     if (!vchSecret.SetString(params[1].get_str()))
         throw runtime_error("Invalid alert master key");
     CKey key = vchSecret.GetKey(); // if key is not correct openssl may crash
     if (!key.Sign(Hash(alert.vchMsg.begin(), alert.vchMsg.end()), alert.vchSig))
         throw runtime_error(
             "Unable to sign alert, check alert master key?\n");
 
     // Process alert
     if(!alert.ProcessAlert())
         throw runtime_error(
             "Failed to process alert.\n");
 
     // Relay alert
     {
         LOCK(cs_vNodes);
         BOOST_FOREACH(CNode* pnode, vNodes)
             alert.RelayTo(pnode);
     }
 
     Object result;
     result.push_back(Pair("strStatusBar", alert.strStatusBar));
     result.push_back(Pair("nVersion", alert.nVersion));
     result.push_back(Pair("nMinVer", alert.nMinVer));
     result.push_back(Pair("nMaxVer", alert.nMaxVer));
     result.push_back(Pair("nPriority", alert.nPriority));
     result.push_back(Pair("nID", alert.nID));
     if (alert.nCancel > 0)
         result.push_back(Pair("nCancel", alert.nCancel));
     return result;
 }