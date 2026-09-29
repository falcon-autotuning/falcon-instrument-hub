vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/instrument-domain
    REF v${VERSION}
    SHA512 23e2fb1914197e932d0667f8c5b022e5be62af452edd871a13ea1498c892be02ea7cf3176819ffdcf7adfeaf25329eb607fa4bee7c51ccb09a805ec825281dd2
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
