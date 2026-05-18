vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/falcon-database
    REF v${VERSION}
    SHA512 e05d6c7f47053cf7487f41c4944c35f68a3b83465327fb85b967dc3fedc9365c3a79fe60cde2be5d8a9c439a9cd2b7390155311d95d37892eeab0a067aa6d3bc
)
vcpkg_cmake_configure(
    SOURCE_PATH "${SOURCE_PATH}"
)
vcpkg_cmake_install()
vcpkg_cmake_config_fixup()
file(INSTALL "${SOURCE_PATH}/LICENSE"
     DESTINATION "${CURRENT_PACKAGES_DIR}/share/${PORT}"
     RENAME copyright)
vcpkg_copy_pdbs()
