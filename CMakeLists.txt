cmake_minimum_required(VERSION 3.20)
project(
  falcon-instrument-hub-deps
  VERSION 1.0.0
  DESCRIPTION "Installs C library dependencies for falcon-instrument-hub via vcpkg"
  LANGUAGES CXX C
)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

include(CTest)

find_package(falcon-core CONFIG REQUIRED)
find_package(nlohmann_json CONFIG REQUIRED)
find_package(nlohmann_json_schema_validator CONFIG REQUIRED)
find_package(yaml-cpp CONFIG REQUIRED)

set(WIREMAP_SCHEMA_FILE "${CMAKE_SOURCE_DIR}/schemas/wiremap.schema.json")

if(NOT EXISTS "${WIREMAP_SCHEMA_FILE}")
  message(FATAL_ERROR "Required schema file does not exist: ${WIREMAP_SCHEMA_FILE}")
endif()

file(READ "${WIREMAP_SCHEMA_FILE}" WIREMAP_SCHEMA)

function(validate_json_schema schema_name schema_contents)
  string(
    JSON schema_type
    ERROR_VARIABLE json_error
    TYPE
    "${schema_contents}"
  )

  if(json_error)
    message(FATAL_ERROR "${schema_name} is not valid JSON: ${json_error}")
  endif()

  if(NOT schema_type STREQUAL "OBJECT")
    message(FATAL_ERROR "${schema_name} must contain a JSON object, got: ${schema_type}")
  endif()
endfunction()

validate_json_schema("Wiremap schema" "${WIREMAP_SCHEMA}")

set_property(
  DIRECTORY
  APPEND
  PROPERTY CMAKE_CONFIGURE_DEPENDS "${WIREMAP_SCHEMA_FILE}"
)

configure_file(
  "${CMAKE_SOURCE_DIR}/src/embedded_schemas.cpp.in"
  "${CMAKE_BINARY_DIR}/embedded_schemas.cpp"
  @ONLY
)

add_executable(validate-wiremap-config
  src/wiremap_config_validator.cpp
  "${CMAKE_BINARY_DIR}/embedded_schemas.cpp"
)

target_link_libraries(validate-wiremap-config
  PRIVATE
    nlohmann_json::nlohmann_json
    nlohmann_json_schema_validator::validator
    yaml-cpp::yaml-cpp
)

install(
  TARGETS validate-wiremap-config
  RUNTIME DESTINATION bin
)

install(
  FILES "${WIREMAP_SCHEMA_FILE}"
  DESTINATION share/falcon-instrument-hub/schemas
)

if(BUILD_TESTING)
  add_test(
    NAME validate-wiremap-valid-fixture
    COMMAND validate-wiremap-config
            "${CMAKE_SOURCE_DIR}/test_data/2-dot-1-chargesensor-wiremap.yml"
  )

  foreach(invalid_fixture
      duplicate-logical-name
      duplicate-physical-endpoint
      invalid-name
      missing-index
      unknown-field
  )
    add_test(
      NAME "validate-wiremap-invalid-${invalid_fixture}"
      COMMAND validate-wiremap-config
              "${CMAKE_SOURCE_DIR}/tests/wiremap/${invalid_fixture}.yml"
    )
    set_tests_properties(
      "validate-wiremap-invalid-${invalid_fixture}"
      PROPERTIES WILL_FAIL TRUE
    )
  endforeach()
endif()
