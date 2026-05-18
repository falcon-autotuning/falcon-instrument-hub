#include <instrument-script-server/plugin/PluginInterface.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>

static const int MOCK_PLUGIN_MAX_PAYLOAD =
    PLUGIN_MAX_STRING_LEN - 1; // since null terminator takes 1 char
static const int MAX_CHANNEL = 32;
static const int MIN_CHANNEL = 1;
static const int NULL_CHANNEL = -1;
static const int NULL_PARAM_INDEX = -1;
static const float NULL_VOLTAGE = 0.0;
static const char *SET = "SET_VOLTAGE";
static const char *GET = "GET_VOLTAGE";
static const char *RESET = "RESET";
static const char *VOLTAGE_IO_NAME = "voltage";
static const char *ANALOG_IO_NAME = "analog";
static const int PLUGIN_INITIALIZATION_ERROR = -1;
static const int MISSING_PARAMETERS_ERROR = -2;
static const int INVALID_PARAMETER_TYPE_ERROR = -3;
static const int CHANNEL_OUT_OF_RANGE_ERROR = -4;
static const int UNKNOWN_COMMAND_ERROR = -5;

static float g_stored_voltage[MAX_CHANNEL - MIN_CHANNEL] = {NULL_VOLTAGE};
static int g_initialized = 0;
// Produces the matching array index for a given analog channel from config
static int getArrayIndex(int channel) {
  if (channel < MIN_CHANNEL || channel > MAX_CHANNEL) {
    return NULL_CHANNEL; // Invalid channel
  }
  return channel - MIN_CHANNEL; // Convert to 0-based index
}
// Copies a message into the target buffer with proper null-termination,
// ensuring it does not exceed the maximum payload size
static void payloadCopy(char *target, char *message) {
  strncpy(target, message, MOCK_PLUGIN_MAX_PAYLOAD);
  target[MOCK_PLUGIN_MAX_PAYLOAD] = '\0'; // Ensure null-termination
}
// Sets an error message and code in the response struct, and returns the error
// code
static int setPluginError(PluginResponse *resp, const char *msg,
                          int error_code) {
  strncpy(resp->error_message, msg, MOCK_PLUGIN_MAX_PAYLOAD);
  resp->error_code = error_code;
  return error_code;
}
// Helper to check parameter count and set error if invalid
// cmd: the command being processed
// resp: the response struct to populate in case of error
// expected: the expected number of parameters
// desc: a description of the expected parameters for error messages
// error_code: the error code to set in case of parameter count mismatch
static int check_param_count(const PluginCommand *cmd, const int expected,
                             PluginResponse *resp) {
  if (cmd->param_count == expected) {
    return 0;
  }
  char msg[MOCK_PLUGIN_MAX_PAYLOAD];
  int count = snprintf(msg, MOCK_PLUGIN_MAX_PAYLOAD,
                       "Invalid number of parameters, found %d but expected %d",
                       cmd->param_count, expected);
  if (count < 0 || count >= MOCK_PLUGIN_MAX_PAYLOAD) {
    // Handle snprintf error or truncation if needed
    payloadCopy(msg, "Parameters names too long");
  }
  return setPluginError(resp, msg, MISSING_PARAMETERS_ERROR);
}
// Finds the index of the parameter with the given name.
// Returns 0 on success and sets *out_index, or returns error_code and sets
// error in resp.
static int get_param_index(const PluginCommand *cmd, const char *param_name,
                           int out_index, PluginResponse *resp) {
  for (uint32_t i = 0; i < cmd->param_count; i++) {
    if (strcmp(cmd->params[i].name, param_name) == 0) {
      out_index = (int)i;
      return 0;
    }
  }
  char msg[MOCK_PLUGIN_MAX_PAYLOAD];
  snprintf(msg, MOCK_PLUGIN_MAX_PAYLOAD, "No %s parameter found", param_name);
  return setPluginError(resp, msg, MISSING_PARAMETERS_ERROR);
}
// Checks that the parameter at cmd->params[idx] has the expected type.
// Returns 0 if the type matches, or sets an error in resp and returns
// error_code const char *param_name is used for error messages to indicate
// which parameter had the wrong type. idx is the index of the parameter to
// check in cmd->params expected_type is the expected PARAM_TYPE_* value for the
// parameter
static int check_param_type(const PluginCommand *cmd, const char *param_name,
                            const int idx, PluginResponse *resp,
                            const int expected_type) {
  if (cmd->params[idx].value.type != expected_type) {
    char msg[MOCK_PLUGIN_MAX_PAYLOAD];
    snprintf(msg, MOCK_PLUGIN_MAX_PAYLOAD, "%s parameter has unsupported type",
             param_name);
    return setPluginError(resp, msg, INVALID_PARAMETER_TYPE_ERROR);
  }
  return 0;
}
// Handler for the SET_VOLTAGE command. Expects parameters "voltage" (float) and
// "analog" (int). Stores the voltage for the specified channel.
static int handle_set(const PluginCommand *cmd, PluginResponse *resp) {
  int voltage_idx, channel_idx;
  if (check_param_count(cmd, 2, resp) |
      get_param_index(cmd, VOLTAGE_IO_NAME, voltage_idx, resp) |
      get_param_index(cmd, ANALOG_IO_NAME, channel_idx, resp) |
      check_param_type(cmd, VOLTAGE_IO_NAME, voltage_idx, resp,
                       PARAM_TYPE_FLOAT) |
      check_param_type(cmd, ANALOG_IO_NAME, channel_idx, resp,
                       PARAM_TYPE_INT32)) {
    return resp->error_code;
  }
  float voltage = cmd->params[voltage_idx].value.value.f_val;
  int channel = cmd->params[channel_idx].value.value.i32_val;
  int index = getArrayIndex(channel);
  if (index == NULL_CHANNEL) {
    return setPluginError(resp, "Channel out of range (must be 1-32)",
                          CHANNEL_OUT_OF_RANGE_ERROR);
  }
  g_stored_voltage[index] = voltage;
  resp->success = true;
  snprintf(resp->text_response, MOCK_PLUGIN_MAX_PAYLOAD,
           "Channel %d voltage set to %.6f V", channel, voltage);
  return resp->error_code;
}
// Handler for the GET_VOLTAGE command. Expects parameter "analog" (int)
// analyzes the specified channel and returns the stored voltage value.
static int handle_get(const PluginCommand *cmd, PluginResponse *resp) {
  int channel_idx;
  if (check_param_count(cmd, 1, resp) |
      get_param_index(cmd, ANALOG_IO_NAME, channel_idx, resp) |
      check_param_type(cmd, ANALOG_IO_NAME, channel_idx, resp,
                       PARAM_TYPE_INT32)) {
    return resp->error_code;
  }
  int channel = cmd->params[channel_idx].value.value.i32_val;
  int index = getArrayIndex(channel);
  if (index == NULL_CHANNEL) {
    return setPluginError(resp, "Channel out of range (must be 1-32)",
                          CHANNEL_OUT_OF_RANGE_ERROR);
  }
  resp->success = true;
  resp->return_value.type = PARAM_TYPE_FLOAT;
  resp->return_value.value.d_val = g_stored_voltage[index];
  snprintf(resp->text_response, MOCK_PLUGIN_MAX_PAYLOAD, "%.6f",
           g_stored_voltage[index]);
  return resp->error_code;
}
// Handler for the RESET command. Expects no parameters. Resets all stored
// voltages to 0.0 V.
static int handle_reset(const PluginCommand *cmd, PluginResponse *resp) {
  if (check_param_count(cmd, 0, resp)) {
    return resp->error_code;
  }
  for (int i = 0; i < MAX_CHANNEL - MIN_CHANNEL; ++i) {
    g_stored_voltage[i] = NULL_VOLTAGE;
  }
  resp->success = true;
  payloadCopy(resp->text_response, "All channel voltages reset to 0.0 V");
  return resp->error_code;
}

INSTRUMENT_PLUGIN_API PluginMetadata plugin_get_metadata(void) {
  PluginMetadata meta = {};
  meta.api_version = INSTRUMENT_PLUGIN_API_VERSION;
  payloadCopy(meta.name, "Mock Voltage Source");
  payloadCopy(meta.version, "1.0.0");
  payloadCopy(meta.protocol_type, "MockVoltageSource");
  payloadCopy(meta.description,
              "Mock voltage source that stores and retrieves voltage values");
  return meta;
}

INSTRUMENT_PLUGIN_API int32_t plugin_initialize(const PluginConfig *config) {
  // Initialize with zero voltage
  for (int i = 0; i < MAX_CHANNEL - MIN_CHANNEL; ++i) {
    g_stored_voltage[i] = NULL_VOLTAGE;
  }
  g_initialized = 1;
  (void)config; // Unused in this simple mock
  return 0;
}

INSTRUMENT_PLUGIN_API int32_t plugin_execute_command(const PluginCommand *cmd,
                                                     PluginResponse *resp) {
  payloadCopy(resp->command_id, cmd->id);
  payloadCopy(resp->instrument_name, cmd->instrument_name);
  resp->success = false;
  resp->error_code = 0;
  resp->binary_response_size = 0;
  resp->has_large_data = false;
  if (!g_initialized) {
    return setPluginError(resp, "Plugin not initialized",
                          PLUGIN_INITIALIZATION_ERROR);
  }
  if (strcmp(cmd->verb, SET) == 0) {
    return handle_set(cmd, resp);
  } else if (strcmp(cmd->verb, GET) == 0) {
    return handle_get(cmd, resp);
  } else if (strcmp(cmd->verb, RESET) == 0) {
    return handle_reset(cmd, resp);
  } else {
    return setPluginError(resp, "Unknown command", UNKNOWN_COMMAND_ERROR);
  }
}

INSTRUMENT_PLUGIN_API void plugin_shutdown(void) { g_initialized = 0; }
