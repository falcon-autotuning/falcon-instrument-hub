vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/instrument-script-server
    REF v${VERSION}
    SHA512 86e9265a921e8b3541c9cf856466a09e4503814a0ab79ae709adb8844129a54cfaef94cfc5801a3982cb5987f7f8874770b2ee4bd0af4b8194ea9a988a155ba9
)

vcpkg_cmake_configure(
    SOURCE_PATH "${SOURCE_PATH}"
    OPTIONS
        -DBUILD_CLI=ON
)

vcpkg_cmake_install()
vcpkg_cmake_config_fixup(PACKAGE_NAME instrument-script-server CONFIG_PATH share/instrument-script-server)

file(INSTALL "${SOURCE_PATH}/LICENSE"
     DESTINATION "${CURRENT_PACKAGES_DIR}/share/${PORT}"
     RENAME copyright)

file(REMOVE_RECURSE "${CURRENT_PACKAGES_DIR}/debug/include")
file(REMOVE_RECURSE "${CURRENT_PACKAGES_DIR}/debug/share")
file(REMOVE_RECURSE "${CURRENT_PACKAGES_DIR}/share/doc/instrument-script-server/assets/icons")

vcpkg_copy_pdbs()
