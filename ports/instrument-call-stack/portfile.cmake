vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/instrument-call-stack
    REF v${VERSION}
    SHA512 9a17ee7c1612df880e81373a243ad4dd962c2f407f3d29b1aba583d344dde7649bc4a984a9c4b240192a6c49450f925493e8ccc884fb7abe6c5d86a358f26fa1
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
