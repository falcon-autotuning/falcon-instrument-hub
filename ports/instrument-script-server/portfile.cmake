# Use the local workspace source when available, otherwise download from GitHub.
# This avoids needing a new GitHub release every time the ISS changes locally.
get_filename_component(WORKSPACE_ROOT "${CURRENT_PORT_DIR}/../../.." ABSOLUTE)
set(_LOCAL_ISS_DIR "${WORKSPACE_ROOT}/instrument-script-server")
set(_LOCAL_ISS_CMAKELISTS "${_LOCAL_ISS_DIR}/CMakeLists.txt")

if(EXISTS "${_LOCAL_ISS_CMAKELISTS}")
  message(STATUS "instrument-script-server: using local workspace source at ${_LOCAL_ISS_DIR}")
  set(SOURCE_PATH "${_LOCAL_ISS_DIR}")
else()
  message(STATUS "instrument-script-server: local workspace not found, downloading v${VERSION} from GitHub")
  vcpkg_from_github(
        OUT_SOURCE_PATH SOURCE_PATH
        REPO falcon-autotuning/instrument-script-server
        REF v${VERSION}
        SHA512 a8164fcba4fec125b74b8262aa62a15ce4fd75e8d333e3ff3697f367f08f6f61272f3b85e9315716f59cd828d0b49afa0a245d20b5c01440a5b3454bc3f94879
  )
endif()

vcpkg_cmake_configure(
    SOURCE_PATH "${SOURCE_PATH}"
    OPTIONS
        -DBUILD_CLI=ON
)

vcpkg_cmake_install()
vcpkg_cmake_config_fixup()

file(INSTALL "${SOURCE_PATH}/LICENSE"
     DESTINATION "${CURRENT_PACKAGES_DIR}/share/${PORT}"
     RENAME copyright)

file(REMOVE_RECURSE "${CURRENT_PACKAGES_DIR}/debug/include")
file(REMOVE_RECURSE "${CURRENT_PACKAGES_DIR}/debug/share")
file(REMOVE_RECURSE "${CURRENT_PACKAGES_DIR}/share/doc/instrument-script-server/assets/icons")

vcpkg_copy_pdbs()
