#include <cerrno>
#include <cmath>
#include <cstdint>
#include <cstdlib>
#include <iostream>
#include <stdexcept>
#include <string>
#include <unordered_map>
#include <vector>

#include <nlohmann/json-schema.hpp>
#include <nlohmann/json.hpp>
#include <yaml-cpp/yaml.h>

namespace hub_schema {
extern const char WIREMAP_SCHEMA[];
}

namespace {

using Json = nlohmann::json;
using JsonPointer = Json::json_pointer;
using JsonValidator = nlohmann::json_schema::json_validator;

struct ValidationIssue {
  std::string path;
  std::string message;
};

bool try_parse_integer(const std::string &text, std::int64_t &result) {
  if (text.empty()) {
    return false;
  }

  char *end = nullptr;
  errno = 0;
  const long long value = std::strtoll(text.c_str(), &end, 10);

  if (errno == ERANGE || end == text.c_str() || end == nullptr ||
      *end != '\0') {
    return false;
  }

  result = static_cast<std::int64_t>(value);
  return true;
}

bool try_parse_number(const std::string &text, double &result) {
  if (text.empty() || text.find_first_of(".eE") == std::string::npos) {
    return false;
  }

  char *end = nullptr;
  errno = 0;
  const double value = std::strtod(text.c_str(), &end);

  if (errno == ERANGE || end == text.c_str() || end == nullptr ||
      *end != '\0' || !std::isfinite(value)) {
    return false;
  }

  result = value;
  return true;
}

Json yaml_scalar_to_json(const YAML::Node &node) {
  const std::string value = node.Scalar();
  const std::string tag = node.Tag();

  if (tag == "tag:yaml.org,2002:null" || tag == "!!null") {
    return nullptr;
  }
  if (tag == "tag:yaml.org,2002:str" || tag == "!!str" || tag == "!") {
    return value;
  }
  if (tag == "tag:yaml.org,2002:bool" || tag == "!!bool") {
    return node.as<bool>();
  }
  if (tag == "tag:yaml.org,2002:int" || tag == "!!int") {
    return node.as<std::int64_t>();
  }
  if (tag == "tag:yaml.org,2002:float" || tag == "!!float") {
    return node.as<double>();
  }

  if (value == "null" || value == "Null" || value == "NULL" ||
      value == "~") {
    return nullptr;
  }
  if (value == "true" || value == "True" || value == "TRUE") {
    return true;
  }
  if (value == "false" || value == "False" || value == "FALSE") {
    return false;
  }

  std::int64_t integer_value = 0;
  if (try_parse_integer(value, integer_value)) {
    return integer_value;
  }

  double number_value = 0.0;
  if (try_parse_number(value, number_value)) {
    return number_value;
  }

  return value;
}

Json yaml_to_json(const YAML::Node &node) {
  if (!node || node.IsNull()) {
    return nullptr;
  }

  if (node.IsScalar()) {
    return yaml_scalar_to_json(node);
  }

  if (node.IsSequence()) {
    Json result = Json::array();
    for (const YAML::Node &entry : node) {
      result.push_back(yaml_to_json(entry));
    }
    return result;
  }

  if (node.IsMap()) {
    Json result = Json::object();
    for (const auto &entry : node) {
      if (!entry.first.IsScalar()) {
        throw std::runtime_error("YAML object keys must be scalar strings");
      }
      result[entry.first.as<std::string>()] = yaml_to_json(entry.second);
    }
    return result;
  }

  throw std::runtime_error("Unsupported YAML node type");
}

class ErrorHandler final : public nlohmann::json_schema::basic_error_handler {
public:
  explicit ErrorHandler(std::vector<ValidationIssue> &issues)
      : issues_(issues) {}

  void error(const JsonPointer &pointer, const Json &instance,
             const std::string &message) override {
    nlohmann::json_schema::basic_error_handler::error(pointer, instance,
                                                      message);
    issues_.push_back({pointer.to_string(), message});
  }

private:
  std::vector<ValidationIssue> &issues_;
};

void validate_schema(const Json &document,
                     std::vector<ValidationIssue> &issues) {
  Json schema = Json::parse(hub_schema::WIREMAP_SCHEMA);
  JsonValidator validator;
  validator.set_root_schema(schema);

  ErrorHandler handler(issues);
  validator.validate(document, handler);
}

std::string endpoint_key(const Json &entry) {
  const Json &instrument = entry.at("instrument");
  return instrument.at("name").get<std::string>() + "." +
         instrument.at("channel_name").get<std::string>() + "." +
         std::to_string(instrument.at("index").get<int>());
}

void validate_unique_entries(const Json &document,
                             std::vector<ValidationIssue> &issues) {
  std::unordered_map<std::string, std::size_t> logical_names;
  std::unordered_map<std::string, std::size_t> physical_endpoints;
  const Json &wiremap = document.at("wiremap");

  for (std::size_t index = 0; index < wiremap.size(); ++index) {
    const Json &entry = wiremap.at(index);
    const std::string logical_name = entry.at("name").get<std::string>();
    const std::string physical_endpoint = endpoint_key(entry);

    const auto logical_inserted = logical_names.emplace(logical_name, index);
    if (!logical_inserted.second) {
      issues.push_back({"/wiremap/" + std::to_string(index) + "/name",
                        "duplicate logical connection name '" + logical_name +
                            "'; first used at /wiremap/" +
                            std::to_string(logical_inserted.first->second)});
    }

    const auto endpoint_inserted =
        physical_endpoints.emplace(physical_endpoint, index);
    if (!endpoint_inserted.second) {
      issues.push_back(
          {"/wiremap/" + std::to_string(index) + "/instrument",
           "duplicate physical instrument endpoint '" + physical_endpoint +
               "'; first used at /wiremap/" +
               std::to_string(endpoint_inserted.first->second)});
    }
  }
}

std::vector<ValidationIssue> validate_wiremap_file(const std::string &path) {
  std::vector<ValidationIssue> issues;

  try {
    const YAML::Node yaml_document = YAML::LoadFile(path);
    const Json document = yaml_to_json(yaml_document);

    validate_schema(document, issues);
    if (issues.empty()) {
      validate_unique_entries(document, issues);
    }
  } catch (const YAML::BadFile &error) {
    issues.push_back({"", "unable to open YAML file '" + path +
                              "': " + error.what()});
  } catch (const YAML::ParserException &error) {
    issues.push_back({"", std::string("YAML parse error: ") + error.what()});
  } catch (const YAML::Exception &error) {
    issues.push_back({"", std::string("YAML processing error: ") +
                              error.what()});
  } catch (const Json::parse_error &error) {
    issues.push_back({"", std::string("embedded wiremap schema is invalid: ") +
                              error.what()});
  } catch (const Json::exception &error) {
    issues.push_back({"", std::string("JSON error: ") + error.what()});
  } catch (const std::exception &error) {
    issues.push_back({"", std::string("validation error: ") + error.what()});
  }

  return issues;
}

} // namespace

int main(int argc, char *argv[]) {
  if (argc != 2) {
    std::cerr << "Usage: " << argv[0] << " <wiremap.yaml>\n";
    return 1;
  }

  const std::vector<ValidationIssue> issues = validate_wiremap_file(argv[1]);
  if (issues.empty()) {
    std::cout << "Validation succeeded.\n";
    return 0;
  }

  std::cout << "Validation failed:\n";
  for (const auto &issue : issues) {
    std::cout << "  - " << issue.path << ": " << issue.message << "\n";
  }
  return 2;
}
