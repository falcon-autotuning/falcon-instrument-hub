vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/instrument-log
    REF v${VERSION}
    SHA512 e41bd962f3a4de175d754966a07d1fe914bd34c7c79fbd7634436554d3b79353046bbba48f529c3368a42bead0481bc7e2e6a5484b5137578bcdb3b26a2821ce
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
