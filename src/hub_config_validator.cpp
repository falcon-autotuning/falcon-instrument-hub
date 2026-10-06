#include <iostream>
#include <yaml-config-validator.hpp>

namespace hub {
extern const char HUB_SCHEMA[];
}

int main(int argc, char *argv[]) {
  if (argc != 2) {
    std::cerr << "Usage: " << argv[0] << " <input.yaml>\n";
    return 1;
  }
  auto result = validator::validate_yaml_file(argv[1], hub::HUB_SCHEMA,
                                              "hub configuration");
  for (const auto &warn : result.warnings) {
    std::cout << "  - " << warn << "\n";
  }
  if (result.valid) {
    std::cout << "Validation succeeded.\n";
    return 0;
  }
  std::cout << "Validation failed:\n";
  for (const auto &err : result.errors) {
    std::cout << "  - " << err.path << ": " << err.message << "\n";
  }
  return 2;
}
