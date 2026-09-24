vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/instrument-script-server
    REF v${VERSION}
    SHA512 b6681d6e550d2870b8c652dbad58b39c198547828d3318fd7976013ccb346644c076a9e1541cce15a7eb82fadf3a31a61f378163f59231cb88fcd960fdf2a3b8
    PATCHES
      main.patch # gives us access to proto files
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
