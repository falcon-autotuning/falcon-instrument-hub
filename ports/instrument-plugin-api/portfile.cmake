vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/instrument-plugin-api
    REF v${VERSION}
    SHA512 f72c0b014ecc65ed962883a57bb3c72a5fb2888930940301a7e2455af0ee95c4c6e488e6e74f58568a5d19dc282df3f859595cf0ea9d2e8d3c2992d02a452e2f
)

if("plugin" IN_LIST FEATURES)
  set(INSTRUMENT_PLUGIN_ENABLE_PLUGIN ON)
endif()

if("host" IN_LIST FEATURES)
  set(INSTRUMENT_PLUGIN_ENABLE_HOST ON)
endif()

vcpkg_cmake_configure(
    SOURCE_PATH "${SOURCE_PATH}"
    OPTIONS
        -DINSTRUMENT_PLUGIN_ENABLE_PLUGIN=${INSTRUMENT_PLUGIN_ENABLE_PLUGIN}
        -DINSTRUMENT_PLUGIN_ENABLE_HOST=${INSTRUMENT_PLUGIN_ENABLE_HOST}
)

vcpkg_cmake_install()
vcpkg_cmake_config_fixup()

set(_config_file "${CURRENT_PACKAGES_DIR}/share/${PORT}/${PORT}-config.cmake")
file(APPEND "${_config_file}" [=[

# Compatibility aliases for consumers written against earlier target names.
if(TARGET instrument-plugin-api::instrument-plugin-api-plugin AND NOT TARGET instrument-plugin-api::plugin)
  add_library(instrument-plugin-api::plugin ALIAS instrument-plugin-api::instrument-plugin-api-plugin)
endif()

if(TARGET instrument-plugin-api::instrument-plugin-api-plugin AND NOT TARGET instrument-plugin-api::instrument-plugin-api)
  add_library(instrument-plugin-api::instrument-plugin-api ALIAS instrument-plugin-api::instrument-plugin-api-plugin)
endif()

if(TARGET instrument-plugin-api::instrument-plugin-api-host AND NOT TARGET instrument-plugin-api::host)
  add_library(instrument-plugin-api::host ALIAS instrument-plugin-api::instrument-plugin-api-host)
endif()
]=])

file(INSTALL "${SOURCE_PATH}/LICENSE"
     DESTINATION "${CURRENT_PACKAGES_DIR}/share/${PORT}"
     RENAME copyright)

file(REMOVE_RECURSE "${CURRENT_PACKAGES_DIR}/debug/include")
file(REMOVE_RECURSE "${CURRENT_PACKAGES_DIR}/debug/share")

vcpkg_copy_pdbs()
