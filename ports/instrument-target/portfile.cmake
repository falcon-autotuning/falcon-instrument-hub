vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/instrument-target
    REF v${VERSION}
    SHA512 e83dc11e9f5f369d42dd6268e948a8fe58810d1b314726c8abb33b04061aa5f785735977855fb04c149563df66f06cff531d724d69bb0cd73392ccc3bfaa3bb4
)

set(BUILD_LUA OFF)

if("lua" IN_LIST FEATURES)
  message(STATUS "Feature 'lua' enabled")
  set(BUILD_LUA ON)
endif()

vcpkg_cmake_configure(
    SOURCE_PATH "${SOURCE_PATH}"
    OPTIONS
        -DBUILD_TESTS=OFF
        -DBUILD_LUA=${BUILD_LUA}
)

vcpkg_cmake_install()

vcpkg_cmake_config_fixup(
    CONFIG_PATH share/${PORT}
)

file(INSTALL "${SOURCE_PATH}/LICENSE"
     DESTINATION "${CURRENT_PACKAGES_DIR}/share/${PORT}"
     RENAME copyright)

file(REMOVE_RECURSE "${CURRENT_PACKAGES_DIR}/debug/include")
file(REMOVE_RECURSE "${CURRENT_PACKAGES_DIR}/debug/share")

vcpkg_copy_pdbs()

set(VCPKG_POLICY_SKIP_ABSOLUTE_PATHS_CHECK enabled)
