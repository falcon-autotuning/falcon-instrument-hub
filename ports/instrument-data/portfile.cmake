vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/instrument-data
    REF v${VERSION}
    SHA512 3af018b972d504d9efb3f9d02a2eb2ae9111d97ea2f5f90a32898b2b34dfbf62a08d094069aa0535fe4a18ae140727670245cd052733ed3cecf764e63b6ce206
)

vcpkg_cmake_configure(
    SOURCE_PATH "${SOURCE_PATH}"
    OPTIONS
        -DBUILD_TESTS=OFF
)

vcpkg_cmake_install()
vcpkg_cmake_config_fixup()

file(INSTALL "${SOURCE_PATH}/LICENSE"
     DESTINATION "${CURRENT_PACKAGES_DIR}/share/${PORT}"
     RENAME copyright)

file(REMOVE_RECURSE "${CURRENT_PACKAGES_DIR}/debug/include")
file(REMOVE_RECURSE "${CURRENT_PACKAGES_DIR}/debug/share")

vcpkg_copy_pdbs()

set(VCPKG_POLICY_SKIP_ABSOLUTE_PATHS_CHECK enabled)
