vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/teal-api-gen
    REF v${VERSION}
    SHA512 93d1b26823ee23c4cdc494852faab319cebc4408c539d987ec83c9e31028eb079c7d65b46217782732ea01c08e3c76682acf7981d712e3a9c95cb231e92f9718
)
vcpkg_cmake_configure(
    SOURCE_PATH "${SOURCE_PATH}"
    OPTIONS
        -DCMAKE_BUILD_TYPE=Release
)
vcpkg_cmake_install()
vcpkg_cmake_config_fixup()
file(INSTALL "${SOURCE_PATH}/LICENSE"
     DESTINATION "${CURRENT_PACKAGES_DIR}/share/${PORT}"
     RENAME copyright)
vcpkg_copy_pdbs()
