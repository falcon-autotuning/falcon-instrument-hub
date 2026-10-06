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

find_package(yaml-config-validator CONFIG REQUIRED)

# Embedded JSON schemas
set(HUB_SCHEMA_FILE
    "${CMAKE_SOURCE_DIR}/schemas/hub.schema.json")

foreach(schema_file
    HUB_SCHEMA_FILE
)
  if(NOT EXISTS "${${schema_file}}")
    message(FATAL_ERROR
      "Required schema file does not exist: ${${schema_file}}")
  endif()
endforeach()

file(READ
  "${HUB_SCHEMA_FILE}"
  HUB_SCHEMA
)

function(validate_json_schema schema_name schema_contents)
  string(
    JSON schema_type
    ERROR_VARIABLE json_error
    TYPE
    "${schema_contents}"
  )

  if(json_error)
    message(FATAL_ERROR
      "${schema_name} is not valid JSON: ${json_error}")
  endif()

  if(NOT schema_type STREQUAL "OBJECT")
    message(FATAL_ERROR
      "${schema_name} must contain a JSON object, got: ${schema_type}")
  endif()
endfunction()

validate_json_schema(
  "Hub schema"
  "${HUB_SCHEMA}"
)


foreach(schema_variable
    HUB_SCHEMA
)
  string(LENGTH "${${schema_variable}}" schema_length)

  message(STATUS
    "${schema_variable} size: ${schema_length} bytes")
endforeach()

set_property(
  DIRECTORY
  APPEND
  PROPERTY CMAKE_CONFIGURE_DEPENDS
    "${HUB_SCHEMA_FILE}"
)

configure_file(
  "${CMAKE_SOURCE_DIR}/src/embedded_schemas.cpp.in"
  "${CMAKE_BINARY_DIR}/embedded_schemas.cpp"
  @ONLY
)

add_executable(validate-hub
  src/hub_config_validator.cpp
  ${CMAKE_BINARY_DIR}/embedded_schemas.cpp
)
target_link_libraries(validate-hub
  PRIVATE
    yaml-config-validator::yaml-config-validator
)

install(
  TARGETS validate-hub
  RUNTIME DESTINATION bin
)

install(
  FILES "${HUB_SCHEMA_FILE}"
  DESTINATION share/falcon-instrument-hub/schemas
)

