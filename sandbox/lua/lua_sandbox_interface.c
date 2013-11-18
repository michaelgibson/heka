/* -*- Mode: C; tab-width: 8; indent-tabs-mode: nil; c-basic-offset: 2 -*- */
/* vim: set ts=2 et sw=2 tw=80: */
/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

/// @brief Sandboxed Lua execution @file
#include <stdlib.h>
#include <stdio.h>
#include <ctype.h>
#include <string.h>
#include <lua.h>
#include <lauxlib.h>
#include <lualib.h>
#include <time.h>
#include <lua_sandbox.h>
#include "_cgo_export.h"

////////////////////////////////////////////////////////////////////////////////
/// Calls to Lua
////////////////////////////////////////////////////////////////////////////////
int process_message(lua_sandbox* lsb)
{
    static const char* func_name = "process_message";
    lua_State* lua = lsb_get_lua(lsb);
    if (!lua) return 1;

    if (lsb_pcall_setup(lsb, func_name)) {
        char err[LSB_ERROR_SIZE];
        snprintf(err, LSB_ERROR_SIZE, "%s() function was not found", func_name);
        lsb_terminate(lsb, err);
        return 1;
    }

    if (lua_pcall(lua, 0, 1, 0) != 0) {
        char err[LSB_ERROR_SIZE];
        size_t len = snprintf(err, LSB_ERROR_SIZE, "%s() %s", func_name,
                              lua_tostring(lua, -1));
        if (len >= LSB_ERROR_SIZE) {
          err[LSB_ERROR_SIZE - 1] = 0;
        }
        lsb_terminate(lsb, err);
        return 1;
    }

    if (!lua_isnumber(lua, 1)) {
        char err[LSB_ERROR_SIZE];
        size_t len = snprintf(err, LSB_ERROR_SIZE,
                              "%s() must return a single numeric value", func_name);
        if (len >= LSB_ERROR_SIZE) {
          err[LSB_ERROR_SIZE - 1] = 0;
        }
        lsb_terminate(lsb, err);
        return 1;
    }

    int status = (int)lua_tointeger(lua, 1);
    lua_pop(lua, 1);

    lsb_pcall_teardown(lsb);

    return status;
}

////////////////////////////////////////////////////////////////////////////////
int timer_event(lua_sandbox* lsb, long long ns)
{
    static const char* func_name = "timer_event";
    lua_State* lua = lsb_get_lua(lsb);
    if (!lua) return 1;

    if (lsb_pcall_setup(lsb, func_name)) {
        char err[LSB_ERROR_SIZE];
        snprintf(err, LSB_ERROR_SIZE, "%s() function was not found", func_name);
        lsb_terminate(lsb, err);
        return 1;
    }

    lua_pushnumber(lua, ns);
    if (lua_pcall(lua, 1, 0, 0) != 0) {
        char err[LSB_ERROR_SIZE];
        size_t len = snprintf(err, LSB_ERROR_SIZE, "%s() %s", func_name,
                              lua_tostring(lua, -1));
        if (len >= LSB_ERROR_SIZE) {
          err[LSB_ERROR_SIZE - 1] = 0;
        }
        lsb_terminate(lsb, err);
        return 1;
    }
    lsb_pcall_teardown(lsb);
    lua_gc(lua, LUA_GCCOLLECT, 0);
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
/// Calls from Lua
////////////////////////////////////////////////////////////////////////////////
int read_config(lua_State* lua)
{
    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        luaL_error(lua, "read_config() invalid lightuserdata");
    }
    lua_sandbox* lsb = (lua_sandbox*)luserdata;

    if (lua_gettop(lua) != 1) {
        luaL_error(lua, "read_config() must have a single argument");
    }
    const char* name = luaL_checkstring(lua, 1);

    struct go_lua_read_config_return gr;
    // Cast away constness of the Lua string, the value is not modified
    // and it will save a copy.
    gr = go_lua_read_config(lsb_get_parent(lsb), (char*)name);
    if (gr.r1 == NULL) {
        lua_pushnil(lua);
    } else {
        switch (gr.r0) {
        case 0:
            lua_pushlstring(lua, gr.r1, gr.r2);
            free(gr.r1);
            break;
        case 3:
            lua_pushnumber(lua, *((GoFloat64*)gr.r1));
            break;
        case 4:
            lua_pushboolean(lua, *((GoInt8*)gr.r1));
            break;
        default:
            lua_pushnil(lua);
            break;
        }
    }
    return 1;
}

////////////////////////////////////////////////////////////////////////////////
int read_message(lua_State* lua)
{
    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        luaL_error(lua, "read_message() invalid lightuserdata");
    }
    lua_sandbox* lsb = (lua_sandbox*)luserdata;

    int n = lua_gettop(lua);
    if (n < 1 || n > 3) {
        luaL_error(lua, "read_message() incorrect number of arguments");
    }
    const char* field = luaL_checkstring(lua, 1);
    int fi = luaL_optinteger(lua, 2, 0);
    luaL_argcheck(lua, fi >= 0, 2, "field index must be >= 0");
    int ai = luaL_optinteger(lua, 3, 0);
    luaL_argcheck(lua, ai >= 0, 3, "array index must be >= 0");

    struct go_lua_read_message_return gr;
    // Cast away constness of the Lua string, the value is not modified
    // and it will save a copy.
    gr = go_lua_read_message(lsb_get_parent(lsb), (char*)field, fi, ai);
    if (gr.r1 == NULL) {
        lua_pushnil(lua);
    } else {
        switch (gr.r0) {
        case 0:
            lua_pushlstring(lua, gr.r1, gr.r2);
            free(gr.r1);
            break;
        case 1:
            lua_pushlstring(lua, gr.r1, gr.r2);
            break;
        case 2:
            if (strncmp("Pid", field, 3) == 0
                || strncmp("Severity", field, 8) == 0) {
                lua_pushinteger(lua, *((GoInt32*)gr.r1));
            } else {
                lua_pushnumber(lua, *((GoInt64*)gr.r1));
            }
            break;
        case 3:
            lua_pushnumber(lua, *((GoFloat64*)gr.r1));
            break;
        case 4:
            lua_pushboolean(lua, *((GoInt8*)gr.r1));
            break;
        default:
            lua_pushnil(lua);
            break;
        }
    }
    return 1;
}

////////////////////////////////////////////////////////////////////////////////
int inject_message(lua_State* lua)
{
    static const char* default_type = "txt";
    static const char* default_name = "";
    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        luaL_error(lua, "inject_message() invalid lightuserdata");
    }
    lua_sandbox* lsb = (lua_sandbox*)luserdata;

    void* ud = NULL;
    const char* type = default_type;
    const char* name = default_name;
    switch (lua_gettop(lua)) {
    case 0:
        break;
    case 2:
        name = luaL_checkstring(lua, 2);
        // fallthru
    case 1:
        switch (lua_type(lua, 1)) {
        case LUA_TSTRING:
            type = lua_tostring(lua, 1);
            if (strlen(type) == 0) type = default_type;
            break;
        case LUA_TTABLE:
            type = "";
            if (lsb_output_protobuf(lsb, 1, 0) != 0) {
              luaL_error(lua, "inject_message() cound not encode protobuf - %s",
                         lsb_get_error(lsb));
            }
            break;
        case LUA_TUSERDATA:
            type = lsb_output_userdata(lsb, 1, 0);
            if (!type) {
                luaL_typerror(lua, 1, "circular_buffer");
            }
            break;
        default:
            luaL_typerror(lua, 1, "string, table, or circular_buffer");
            break;
        }
        break;
    default:
        luaL_error(lua, "inject_message() takes a maximum of 2 arguments");
        break;
    }
    size_t len;
    const char* output = lsb_get_output(lsb, &len);

    if (len != 0) {
        int result = go_lua_inject_message(lsb_get_parent(lsb),
                                           (char*)output,
                                           (int)len,
                                           (char*)type,
                                           (char*)name);
        if (result != 0) {
            luaL_error(lua, "inject_message() exceeded MaxMsgLoops");
        }
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int sandbox_init(lua_sandbox* lsb, const char* data_file)
{
    if (!lsb) return 1;

    lsb_add_function(lsb, &read_config, "read_config");
    lsb_add_function(lsb, &read_message, "read_message");
    lsb_add_function(lsb, &inject_message, "inject_message");

    int result = lsb_init(lsb, data_file);
    if (result) return result;

    return 0;
}
